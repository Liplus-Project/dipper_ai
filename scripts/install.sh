#!/usr/bin/env bash
# install.sh — installs dipper_ai on AlmaLinux 9/10
set -euo pipefail

BINARY="./dipper_ai"
INSTALL_DIR="/usr/bin"
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
# Restore correct SELinux file context so systemd can execute the binary.
if command -v restorecon &>/dev/null; then
  restorecon -v "$INSTALL_DIR/dipper_ai"
fi

echo "Creating config directory..."
mkdir -p "$CONF_DIR"
if [[ ! -f "$CONF_DIR/user.conf" ]]; then
  if [[ -f "./user.conf.example" ]]; then
    install -m 0640 ./user.conf.example "$CONF_DIR/user.conf.example"
    echo "  -> Example config installed at $CONF_DIR/user.conf.example"
    echo "     Copy to $CONF_DIR/user.conf and edit before starting."
  fi
fi

# --- Determine DDNS_TIME for systemd timer interval ---
# Read DDNS_TIME (integer minutes) from user.conf.
# Priority: /etc/dipper_ai/user.conf > ./user.conf > default (5 min).
# DDNS_TIME=0 means "no rate-limit gate" — fall back to 5-minute default.
DDNS_TIME_MIN=5
for conf_candidate in "$CONF_DIR/user.conf" "./user.conf"; do
  if [[ -f "$conf_candidate" ]]; then
    v=$(grep -E '^DDNS_TIME=' "$conf_candidate" 2>/dev/null | tail -1 | cut -d= -f2 | sed 's/[[:space:]#].*//')
    if [[ "$v" =~ ^[1-9][0-9]*$ ]]; then
      DDNS_TIME_MIN="$v"
    fi
    break
  fi
done

echo "Installing systemd units (DDNS_TIME=${DDNS_TIME_MIN}min)..."
install -m 0644 ./systemd/dipper_ai.service "$SYSTEMD_DIR/"

# Generate the timer with the interval derived from DDNS_TIME.
# OnBootSec=2min gives the system a short warm-up period after boot.
cat > "$SYSTEMD_DIR/dipper_ai.timer" <<EOF
[Unit]
Description=dipper_ai DDNS manager timer

[Timer]
OnBootSec=2min
OnUnitActiveSec=${DDNS_TIME_MIN}min
Unit=dipper_ai.service

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now dipper_ai.timer

echo "dipper_ai installed successfully."
echo "Status: $(systemctl is-active dipper_ai.timer)"
