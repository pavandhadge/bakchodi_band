#!/usr/bin/env sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this installer with sudo: sudo sh packaging/linux/install.sh"
  exit 1
fi

BIN_SRC="${1:-./bakchodi}"
BIN_DST="/usr/local/bin/bakchodi"
SERVICE="/etc/systemd/system/bakchodi.service"

if [ ! -f "$BIN_SRC" ]; then
  echo "Binary not found: $BIN_SRC"
  echo "Build it first: go build -o bakchodi ./cmd"
  exit 1
fi

install -m 0755 "$BIN_SRC" "$BIN_DST"
mkdir -p /var/lib/bakchodi_band

cat > "$SERVICE" <<EOF
[Unit]
Description=bakchodi_band attention blocker
After=network.target

[Service]
Type=simple
ExecStart=$BIN_DST band
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable bakchodi.service
systemctl restart bakchodi.service

echo "bakchodi_band installed and started."
echo "Check status: systemctl status bakchodi.service"
