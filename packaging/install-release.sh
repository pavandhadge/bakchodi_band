#!/usr/bin/env sh
set -eu

REPO="${BAKCHODI_REPO:-pavandhadge/bakchodi_band}"
VERSION="${BAKCHODI_VERSION:-latest}"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

need_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "Run as root: curl -fsSL <url> | sudo sh"
    exit 1
  fi
}

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
  else
    echo "Install curl or wget first."
    exit 1
  fi
}

asset_url() {
  asset="$1"
  if [ "$VERSION" = "latest" ]; then
    echo "https://github.com/$REPO/releases/latest/download/$asset"
  else
    echo "https://github.com/$REPO/releases/download/$VERSION/$asset"
  fi
}

need_root

os="$(uname -s)"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Unsupported architecture: $arch"; exit 1 ;;
esac

case "$os" in
  Linux)
    asset="bakchodi-linux-$arch"
    installer="packaging/linux/install.sh"
    ;;
  Darwin)
    asset="bakchodi-darwin-$arch"
    installer="packaging/macos/install.sh"
    ;;
  *)
    echo "Unsupported OS: $os"
    exit 1
    ;;
esac

binary="$TMP_DIR/bakchodi"
download "$(asset_url "$asset")" "$binary"
chmod +x "$binary"

script="$TMP_DIR/install.sh"
raw_branch="${BAKCHODI_BRANCH:-main}"
download "https://raw.githubusercontent.com/$REPO/$raw_branch/$installer" "$script"
sh "$script" "$binary"
