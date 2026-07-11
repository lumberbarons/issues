#!/usr/bin/env bash
# Installs the latest issues release binary to $HOME/.local/bin (override
# with INSTALL_DIR). Never uses sudo.
#
#   curl -fsSL https://raw.githubusercontent.com/lumberbarons/issues/main/install.sh | bash
set -euo pipefail

REPO="lumberbarons/issues"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux | darwin) ;;
  *)
    echo "unsupported OS: $os (use: go install github.com/$REPO/cmd/issues@latest)" >&2
    exit 1
    ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *)
    echo "unsupported architecture: $arch (use: go install github.com/$REPO/cmd/issues@latest)" >&2
    exit 1
    ;;
esac

tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
  grep -m1 '"tag_name"' | cut -d'"' -f4)
if [ -z "$tag" ]; then
  echo "cannot resolve the latest release of $REPO" >&2
  exit 1
fi
version="${tag#v}"

archive="issues_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "downloading issues $tag ($os/$arch)..."
curl -fsSL -o "$tmp/$archive" "$base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

(
  cd "$tmp"
  expected=$(grep " $archive\$" checksums.txt | cut -d' ' -f1)
  if [ -z "$expected" ]; then
    echo "no checksum for $archive in checksums.txt" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$archive" | cut -d' ' -f1)
  else
    actual=$(shasum -a 256 "$archive" | cut -d' ' -f1)
  fi
  if [ "$expected" != "$actual" ]; then
    echo "checksum mismatch for $archive" >&2
    exit 1
  fi
)

tar -xzf "$tmp/$archive" -C "$tmp" issues
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/issues" "$INSTALL_DIR/issues"
echo "installed $("$INSTALL_DIR/issues" --version) to $INSTALL_DIR/issues"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "note: $INSTALL_DIR is not on your PATH" ;;
esac
