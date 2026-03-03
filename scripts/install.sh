#!/usr/bin/env bash
# install.sh — installs dipper_ai on AlmaLinux 9/10
set -euo pipefail

BINARY="./dipper_ai"
INSTALL_DIR="/usr/local/bin"
CONF_DIR="/etc/dipper_ai"
SYSTEMD_DIR="/etc/systemd/system"

if [[ $EUID -ne 0 ]]; then
  echo "Run as root" >&2
  exit 1
fi

if [[ ! -f "$BINARY" ]]; then
  echo "Binary not found: $BINARY" >&2
  exit 1
fi

echo "Installing binary..."
install -m 0755 "$BINARY" "$INSTALL_DIR/dipper_ai"

echo "Creating config directory..."
mkdir -p "$CONF_DIR"
if [[ ! -f "$CONF_DIR/user.conf" ]]; then
  if [[ -f "./user.conf.example" ]]; then
    install -m 0640 ./user.conf.example "$CONF_DIR/user.conf.example"
    echo "  -> Example config installed at $CONF_DIR/user.conf.example"
    echo "     Copy to $CONF_DIR/user.conf and edit before starting."
  fi
fi

echo "Installing systemd units..."
install -m 0644 ./systemd/dipper_ai.service "$SYSTEMD_DIR/"
install -m 0644 ./systemd/dipper_ai.timer   "$SYSTEMD_DIR/"

systemctl daemon-reload
systemctl enable --now dipper_ai.timer

echo "dipper_ai installed successfully."
echo "Status: $(systemctl is-active dipper_ai.timer)"
