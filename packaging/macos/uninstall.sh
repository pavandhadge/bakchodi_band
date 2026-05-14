#!/usr/bin/env sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this uninstaller with sudo: sudo sh packaging/macos/uninstall.sh"
  exit 1
fi

PLIST_DST="/Library/LaunchDaemons/com.bakchodi.band.plist"

launchctl bootout system "$PLIST_DST" 2>/dev/null || true
rm -f "$PLIST_DST"
rm -f /usr/local/bin/bakchodi

echo "bakchodi_band service and binary removed."
echo "User data is still in /var/lib/bakchodi_band."
