#!/usr/bin/env bash
# uninstall.sh — removes dipper_ai from the system
set -euo pipefail

INSTALL_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"

if [[ $EUID -ne 0 ]]; then
  echo "Run as root" >&2
  exit 1
fi

echo "Stopping and disabling systemd units..."
systemctl disable --now dipper_ai.timer  2>/dev/null || true
systemctl disable --now dipper_ai.service 2>/dev/null || true

echo "Removing systemd unit files..."
rm -f "$SYSTEMD_DIR/dipper_ai.service"
rm -f "$SYSTEMD_DIR/dipper_ai.timer"
systemctl daemon-reload

echo "Removing binary..."
rm -f "$INSTALL_DIR/dipper_ai"

echo "dipper_ai uninstalled."
echo "Note: /etc/dipper_ai (config and state) was NOT removed."
echo "      Remove manually if no longer needed: rm -rf /etc/dipper_ai"
