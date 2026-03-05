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

# --- Parse time config values from user.conf ---
# Supported formats: 5m, 2h, 1d, 30s, or plain integer (minutes).
# Priority: /etc/dipper_ai/user.conf > ./user.conf > default.
# Returns 0 for unrecognised values.

parse_duration_min() {
  local v="$1"
  if   [[ "$v" =~ ^([0-9]+)d$ ]]; then echo $(( ${BASH_REMATCH[1]} * 1440 ))
  elif [[ "$v" =~ ^([0-9]+)h$ ]]; then echo $(( ${BASH_REMATCH[1]} * 60 ))
  elif [[ "$v" =~ ^([0-9]+)m$ ]]; then echo "${BASH_REMATCH[1]}"
  elif [[ "$v" =~ ^([0-9]+)s$ ]]; then
    local sec="${BASH_REMATCH[1]}"
    echo $(( (sec + 59) / 60 ))   # round up to nearest minute
  elif [[ "$v" =~ ^[0-9]+$ ]];    then echo "$v"  # plain integer = minutes
  else echo "0"                                    # unrecognised → 0
  fi
}

read_conf_value() {
  local key="$1"
  local val=""
  for conf_candidate in "$CONF_DIR/user.conf" "./user.conf"; do
    if [[ -f "$conf_candidate" ]]; then
      val=$(grep -E "^${key}=" "$conf_candidate" 2>/dev/null | tail -1 | cut -d= -f2 | sed 's/[[:space:]#].*//')
      break
    fi
  done
  echo "$val"
}

# --- DDNS_TIME: check/update timer interval (default 5 min) ---
DDNS_TIME_MIN=5
_v=$(read_conf_value "DDNS_TIME")
_parsed=$(parse_duration_min "$_v")
if [[ "$_parsed" =~ ^[1-9][0-9]*$ ]]; then
  DDNS_TIME_MIN="$_parsed"
fi

# --- UPDATE_TIME: keepalive timer interval (default 1440 min = 1 day) ---
# UPDATE_TIME=0 means keepalive is disabled — no keepalive timer is installed.
UPDATE_TIME_MIN=1440
_v=$(read_conf_value "UPDATE_TIME")
_parsed=$(parse_duration_min "$_v")
if [[ "$_parsed" =~ ^[0-9]+$ ]]; then
  UPDATE_TIME_MIN="$_parsed"
fi

echo "Installing systemd units (DDNS_TIME=${DDNS_TIME_MIN}min, UPDATE_TIME=${UPDATE_TIME_MIN}min)..."
install -m 0644 ./systemd/dipper_ai.service "$SYSTEMD_DIR/"
install -m 0644 ./systemd/dipper_ai-keepalive.service "$SYSTEMD_DIR/"

# --- Check/update timer (DDNS_TIME interval) ---
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

# --- Keepalive timer (UPDATE_TIME interval) ---
# Omitted entirely when UPDATE_TIME=0 (keepalive disabled).
if [[ "$UPDATE_TIME_MIN" =~ ^[1-9][0-9]*$ ]]; then
  cat > "$SYSTEMD_DIR/dipper_ai-keepalive.timer" <<EOF
[Unit]
Description=dipper_ai DDNS keepalive timer

[Timer]
OnBootSec=5min
OnUnitActiveSec=${UPDATE_TIME_MIN}min
Unit=dipper_ai-keepalive.service

[Install]
WantedBy=timers.target
EOF
  systemctl enable --now dipper_ai-keepalive.timer
else
  # Disable keepalive timer if it was previously installed.
  systemctl disable --now dipper_ai-keepalive.timer 2>/dev/null || true
fi

systemctl daemon-reload
systemctl enable --now dipper_ai.timer

echo "dipper_ai installed successfully."
echo "Check timer:     $(systemctl is-active dipper_ai.timer)"
if [[ "$UPDATE_TIME_MIN" =~ ^[1-9][0-9]*$ ]]; then
  echo "Keepalive timer: $(systemctl is-active dipper_ai-keepalive.timer)"
else
  echo "Keepalive timer: disabled (UPDATE_TIME=0)"
fi
