#!/usr/bin/env python3
"""Exercise installers against real release assets in an isolated directory."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import zipfile


def verify(assets, powershell="pwsh"):
    manifest = json.loads((assets / 'manifest.json').read_text())
    version = manifest['version']
    for skill in ('klib-development', 'klib-cloud-development'):
        with zipfile.ZipFile(assets / f'{skill}.zip') as archive:
            assert f'{skill}/SKILL.md' in archive.namelist()
            assert f'name: {skill}' in archive.read(f'{skill}/SKILL.md').decode('utf-8')
    windows = platform.system() == 'Windows'
    target = 'windows-amd64' if windows else ('darwin-arm64' if platform.system() == 'Darwin' else 'linux-amd64')
    asset = f'klib_{version}_{target}' + ('.exe' if windows else '')
    with tempfile.TemporaryDirectory(prefix='klib-install-test-') as temporary:
        root = Path(temporary)
        install = root / 'bin with spaces'
        source = root / 'offline'
        source.mkdir()
        for name in (asset, 'SHA256SUMS'):
            shutil.copyfile(assets / name, source / name)
        installer_env = dict(os.environ)
        if windows and powershell.lower() == 'powershell':
            # Do not leak PowerShell 7 module paths into a Windows PowerShell 5.1 child.
            installer_env = {key: value for key, value in installer_env.items() if key.upper() != 'PSMODULEPATH'}
        if windows:
            command = [powershell, '-NoProfile', '-File', str(assets / 'install.ps1'), '-Version', version, '-InstallDir', str(install), '-SourceDir', str(source)]
        else:
            command = ['sh', str(assets / 'install.sh'), '--version', version, '--install-dir', str(install), '--source-dir', str(source)]
        subprocess.run(command, check=True, env=installer_env)
        binary = install / ('klib.exe' if windows else 'klib')
        result = json.loads(subprocess.check_output([str(binary), 'version'], text=True))
        assert result['version'] == version and result['commit'] == manifest['commit'] and result['protocol_version'] == 5, result
        schema = json.loads(subprocess.check_output([str(binary), 'schema'], text=True))
        assert 'version' in schema['commands']
        assert any(command.startswith('docs search ') for command in schema['commands'])
        original = hashlib.sha256(binary.read_bytes()).hexdigest()
        # Reinstallation works; tampering and incomplete downloads cannot replace it.
        subprocess.run(command, check=True, env=installer_env)
        with (source / asset).open('ab') as stream:
            stream.write(b'tampered')
        failed = subprocess.run(command, capture_output=True, text=True, env=installer_env)
        assert failed.returncode != 0 and 'SHA-256 mismatch' in failed.stderr, failed
        assert hashlib.sha256(binary.read_bytes()).hexdigest() == original
        (source / asset).unlink()
        assert subprocess.run(command, capture_output=True, env=installer_env).returncode != 0
        assert hashlib.sha256(binary.read_bytes()).hexdigest() == original
        shutil.copyfile(assets / asset, source / asset)
        (source / 'SHA256SUMS').write_text('')
        assert subprocess.run(command, capture_output=True, env=installer_env).returncode != 0
        assert hashlib.sha256(binary.read_bytes()).hexdigest() == original
    print(f'installer validation passed: {target}')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assets', type=Path, required=True)
    parser.add_argument('--powershell', default='pwsh')
    args = parser.parse_args()
    verify(args.assets.resolve(), args.powershell)
