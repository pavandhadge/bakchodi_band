#!/usr/bin/env sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this uninstaller with sudo: sudo sh packaging/linux/uninstall.sh"
  exit 1
fi

systemctl disable --now bakchodi.service 2>/dev/null || true
rm -f /etc/systemd/system/bakchodi.service
systemctl daemon-reload
rm -f /usr/local/bin/bakchodi

echo "bakchodi_band service and binary removed."
echo "User data is still in /var/lib/bakchodi_band."
