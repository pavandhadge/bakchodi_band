#!/usr/bin/env sh
set -eu

REPO="${BAKCHODI_REPO:-pavandhadge/bakchodi_band}"
VERSION="${BAKCHODI_VERSION:-latest}"
BRANCH="${BAKCHODI_BRANCH:-main}"
BINARY_NAME="bakchodi"
TMP_DIR="$(mktemp -d)"
trap cleanup EXIT

cleanup() {
    rm -rf "$TMP_DIR"
}

download() {
    url="$1"
    out="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$out"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$out" "$url"
    else
        echo "Error: curl or wget required"
        exit 1
    fi
}

usage() {
    cat <<EOF
bakchodi installer

Usage:
  curl -fsSL https://raw.githubusercontent.com/$REPO/$BRANCH/packaging/install.sh | sudo sh

Environment:
  BAKCHODI_REPO=pavandhadge/bakchodi_band  GitHub owner/repo
  BAKCHODI_VERSION=latest                  Release tag or latest
  BAKCHODI_BRANCH=main                     Branch used for macOS plist download
EOF
}

get_asset_name() {
    os="$1"
    arch="$2"

    case "$os" in
        Linux)  echo "bakchodi-linux-$arch" ;;
        Darwin) echo "bakchodi-darwin-$arch" ;;
        *)      echo "" ;;
    esac
}

get_download_url() {
    asset="$1"
    if [ "$VERSION" = "latest" ]; then
        echo "https://github.com/$REPO/releases/latest/download/$asset"
    else
        echo "https://github.com/$REPO/releases/download/$VERSION/$asset"
    fi
}

install_linux() {
    if [ "$(id -u)" -ne 0 ]; then
        echo "Error: Run with sudo: sudo sh install.sh"
        exit 1
    fi

    install -m 0755 "$TMP_DIR/bakchodi" "/usr/local/bin/bakchodi"
    mkdir -p /var/lib/bakchodi_band

    cat > /etc/systemd/system/bakchodi.service <<'EOF'
[Unit]
Description=bakchodi_band attention blocker
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/bakchodi band
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

    if command -v systemctl >/dev/null 2>&1; then
        systemctl daemon-reload
        systemctl enable bakchodi.service
        systemctl restart bakchodi.service
        echo "Installed and started bakchodi.service"
    else
        echo "Installed binary to /usr/local/bin/bakchodi"
        echo "systemd not available - start manually with: /usr/local/bin/bakchodi band"
    fi
}

install_macos() {
    if [ "$(id -u)" -ne 0 ]; then
        echo "Error: Run with sudo: sudo sh install.sh"
        exit 1
    fi

    install -m 0755 "$TMP_DIR/bakchodi" "/usr/local/bin/bakchodi"
    mkdir -p /var/lib/bakchodi_band

    PLIST_SRC="$TMP_DIR/com.bakchodi.band.plist"
    download "https://raw.githubusercontent.com/$REPO/$BRANCH/packaging/macos/com.bakchodi.band.plist" "$PLIST_SRC"
    install -m 0644 "$PLIST_SRC" "/Library/LaunchDaemons/com.bakchodi.band.plist"
    chown root:wheel "/Library/LaunchDaemons/com.bakchodi.band.plist"

    launchctl bootout system "/Library/LaunchDaemons/com.bakchodi.band.plist" 2>/dev/null || true
    launchctl bootstrap system "/Library/LaunchDaemons/com.bakchodi.band.plist"
    launchctl enable system/com.bakchodi.band

    echo "Installed and started bakchodi daemon"
}

test_install() {
    echo "Testing installation..."
    if ! command -v bakchodi >/dev/null 2>&1; then
        echo "Error: bakchodi not found in PATH"
        return 1
    fi

    if ! bakchodi 2>/dev/null | grep -q "bakchodi_band"; then
        echo "Warning: bakchodi binary did not print expected usage text"
    fi

    case "$os" in
        Linux)
            if command -v systemctl >/dev/null 2>&1; then
                systemctl is-active --quiet bakchodi.service
                echo "Test passed: bakchodi.service is active"
            else
                echo "Test skipped: systemd is not available"
            fi
            ;;
        Darwin)
            launchctl print system/com.bakchodi.band >/dev/null
            echo "Test passed: com.bakchodi.band is loaded"
            ;;
    esac
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    usage
    exit 0
fi

os="$(uname -s)"
arch="$(uname -m)"

case "$arch" in
    x86_64|amd64)  arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) echo "Error: Unsupported architecture: $arch"; exit 1 ;;
esac

case "$os" in
    Linux|Darwin) ;;
    *) echo "Error: Unsupported OS: $os"; exit 1 ;;
esac

asset=$(get_asset_name "$os" "$arch")
if [ -z "$asset" ]; then
    echo "Error: No asset for $os-$arch"
    exit 1
fi

echo "Downloading bakchodi for $os ($arch)..."
download "$(get_download_url "$asset")" "$TMP_DIR/bakchodi"
chmod +x "$TMP_DIR/bakchodi"

echo "Installing bakchodi..."
case "$os" in
    Linux) install_linux ;;
    Darwin) install_macos ;;
esac

test_install

echo ""
echo "Installation complete!"
echo "The bakchodi_band service is installed and configured to start automatically."
echo "Try: bakchodi block --url youtube.com"
