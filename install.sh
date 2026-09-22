#!/bin/sh
# Download this script first, then run it with --repo OWNER/REPO --version VERSION.
set -eu
repo=''
version=''
install_dir="${HOME}/.local/bin"
source_dir=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo|--version|--install-dir|--source-dir)
      [ "$#" -ge 2 ] || { echo "Missing value for $1" >&2; exit 2; }
      case "$1" in
        --repo) repo=$2;; --version) version=$2;;
        --install-dir) install_dir=$2;; --source-dir) source_dir=$2;;
      esac
      shift 2;;
    *) echo "Usage: sh install.sh --repo OWNER/REPO --version VERSION [--install-dir DIR] [--source-dir OFFLINE_ASSETS]" >&2; exit 2;;
  esac
done
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9]+([.-][A-Za-z0-9]+)*)?$' || { echo 'Invalid version' >&2; exit 2; }
case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) platform=darwin-arm64;;
  Linux/x86_64) platform=linux-amd64;;
  *) echo 'Supported: macOS ARM64, Linux x64; use install.ps1 for Windows x64.' >&2; exit 2;;
esac
asset="klib_${version}_${platform}"
tmp=$(mktemp -d)
staged=''
trap 'rm -r -- "$tmp"; if [ -n "$staged" ]; then rm -f -- "$staged"; fi' EXIT HUP INT TERM
if [ -n "$source_dir" ]; then
  cp "$source_dir/$asset" "$tmp/$asset"
  cp "$source_dir/SHA256SUMS" "$tmp/SHA256SUMS"
else
  printf '%s\n' "$repo" | grep -Eq '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' || { echo 'Specify --repo OWNER/REPO' >&2; exit 2; }
  base="https://github.com/$repo/releases/download/cli-v$version"
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/$asset" -o "$tmp/$asset"
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
fi
expected=$(awk -v name="$asset" '$2 == name {print $1}' "$tmp/SHA256SUMS")
printf '%s\n' "$expected" | grep -Eq '^[a-f0-9]{64}$' || { echo 'Missing or ambiguous checksum' >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$asset" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
fi
[ "$actual" = "$expected" ] || { echo 'SHA-256 mismatch; existing installation unchanged' >&2; exit 1; }
mkdir -p "$install_dir"
[ ! -L "$install_dir/klib" ] || { echo 'Refusing to replace a symlink' >&2; exit 1; }
staged=$(mktemp "$install_dir/.klib-install.XXXXXX")
cp "$tmp/$asset" "$staged"
chmod 755 "$staged"
"$staged" version
mv -f "$staged" "$install_dir/klib"
staged=''
printf 'Installed: %s/klib\nAdd this directory to PATH if needed: %s\n' "$install_dir" "$install_dir"
