#!/usr/bin/env python3
"""Build standalone CLI release assets from this repository."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = (("darwin", "arm64"), ("linux", "amd64"), ("windows", "amd64"))


def command(*args, cwd=ROOT, **kwargs):
    return subprocess.check_output(args, cwd=cwd, text=True, **kwargs).strip()


def build(output, repo, allow_dirty=False):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo):
        raise ValueError("repo must be OWNER/REPO")
    version = (ROOT / "VERSION").read_text().strip()
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?", version):
        raise ValueError("invalid VERSION")
    dirty = bool(command("git", "status", "--porcelain", "--untracked-files=normal"))
    if dirty and not allow_dirty:
        raise ValueError("release build requires a clean commit; --allow-dirty is for local tests only")
    commit = command("git", "rev-parse", "HEAD")
    output.mkdir(parents=True, exist_ok=False)
    assets, public = output / "assets", output / "public"
    assets.mkdir()
    public.mkdir()
    for name in ("install.sh", "install.ps1", "LICENSE"):
        shutil.copyfile(ROOT / name, public / name)
    readme = (ROOT / "README.md").read_text().replace("@VERSION@", version).replace("@REPO@", repo)
    (public / "README.md").write_text(readme)
    skill = public / "skills/klib-cloud-development"
    skill.mkdir(parents=True)
    shutil.copyfile(ROOT / "skills/klib-cloud-development/SKILL.md", skill / "SKILL.md")
    docs = public / "docs"
    docs.mkdir()
    source_doc = (ROOT / "docs/cli-development.md").read_text()
    source_doc = source_doc.replace("分发安装包与独立 Skill 的构建、发布方式见 [分发说明](cli-distribution.md)。\n\n", "")
    (docs / "cli-development.md").write_text(source_doc)
    licenses = public / "LICENSES"
    licenses.mkdir()
    goroot = Path(command("go", "env", "GOROOT"))
    go_license = goroot / "LICENSE"
    if not go_license.exists():
        go_license = goroot.parent / "LICENSE"  # Homebrew keeps licensing above libexec.
    shutil.copyfile(go_license, licenses / "Go.txt")
    backend = ROOT
    modules = command("go", "list", "-deps", "-f", "{{if .Module}}{{if not .Module.Main}}{{.Module.Path}}{{end}}{{end}}", "./cmd/klib")
    if modules.strip():
        raise ValueError("Review licensing before adding external dependencies")
    (public / "THIRD_PARTY_NOTICES.md").write_text("# Third-party notices\n\nThe CLI uses the Go standard library and runtime. See LICENSES/Go.txt.\n")
    for goos, goarch in TARGETS:
        name = f"klib_{version}_{goos}-{goarch}" + (".exe" if goos == "windows" else "")
        env = dict(os.environ, CGO_ENABLED="0", GOOS=goos, GOARCH=goarch)
        package = "github.com/kzheart/klib-cli/internal/cloudcli"
        subprocess.run(["go", "build", "-trimpath", "-buildvcs=true", "-ldflags", f"-s -w -X {package}.Version={version} -X {package}.Commit={commit}", "-o", str(assets / name), "./cmd/klib"], cwd=backend, env=env, check=True)
    for name in ("install.sh", "install.ps1", "LICENSE", "THIRD_PARTY_NOTICES.md"):
        shutil.copyfile(public / name, assets / name)
    with zipfile.ZipFile(assets / "klib-cloud-development.zip", "w", zipfile.ZIP_DEFLATED) as archive:
        archive.write(skill / "SKILL.md", "klib-cloud-development/SKILL.md")
    with zipfile.ZipFile(assets / "third-party-licenses.zip", "w", zipfile.ZIP_DEFLATED) as archive:
        for path in sorted(licenses.iterdir()):
            archive.write(path, f"LICENSES/{path.name}")
    manifest = {"version": version, "commit": commit, "modified": dirty, "protocol_version": 5, "repository": repo, "platforms": [f"{a}-{b}" for a, b in TARGETS], "prerelease": True}
    for directory in (public, assets):
        (directory / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    sums = "".join(f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n" for p in sorted(assets.iterdir()))
    (assets / "SHA256SUMS").write_text(sums)
    print(output)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--repo", default="kzheart/klib-cli")
    parser.add_argument("--allow-dirty", action="store_true")
    args = parser.parse_args()
    build(args.output.resolve(), args.repo, args.allow_dirty)
