#!/usr/bin/env sh
set -eu

REPO="${BAKCHODI_REPO:-pavandhadge/bakchodi_band}"
BRANCH="${BAKCHODI_BRANCH:-main}"

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: Run with sudo"
    echo "  curl -fsSL https://raw.githubusercontent.com/$REPO/$BRANCH/packaging/install.sh | sudo sh"
    exit 1
fi

echo "Installing bakchodi from $REPO (branch: $BRANCH)..."
curl -fsSL "https://raw.githubusercontent.com/$REPO/$BRANCH/packaging/install.sh" | sh
