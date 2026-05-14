#!/usr/bin/env sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this installer with sudo: sudo sh packaging/macos/install.sh"
  exit 1
fi

BIN_SRC="${1:-./bakchodi}"
BIN_DST="/usr/local/bin/bakchodi"
PLIST_SRC="packaging/macos/com.bakchodi.band.plist"
PLIST_DST="/Library/LaunchDaemons/com.bakchodi.band.plist"

if [ ! -f "$BIN_SRC" ]; then
  echo "Binary not found: $BIN_SRC"
  echo "Build it first: go build -o bakchodi ./cmd"
  exit 1
fi

install -m 0755 "$BIN_SRC" "$BIN_DST"
mkdir -p /var/lib/bakchodi_band
install -m 0644 "$PLIST_SRC" "$PLIST_DST"
chown root:wheel "$PLIST_DST"

launchctl bootout system "$PLIST_DST" 2>/dev/null || true
launchctl bootstrap system "$PLIST_DST"
launchctl enable system/com.bakchodi.band

echo "bakchodi_band installed and started."
echo "Check status: sudo launchctl print system/com.bakchodi.band"
