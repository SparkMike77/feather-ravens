#!/usr/bin/env bash
# Interactive installer for one Raven instance: asks for a topic and a target feed, writes
# /etc/feather-ravens/<slug>.toml, (re)builds and installs the binary, installs the
# raven@.service unit if it isn't there yet, and enables+starts raven@<slug>. Requires sudo -
# it writes under /etc and /usr/local/bin and manages a systemd unit.
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "Run this with sudo - it writes to /etc/feather-ravens, /usr/local/bin, and manages systemd units." >&2
  exit 1
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_DIR=/etc/feather-ravens
BIN_PATH=/usr/local/bin/raven
UNIT_PATH=/etc/systemd/system/raven@.service

read -rp "Topic to watch for (e.g. \"AI\"): " topic
[[ -z "$topic" ]] && { echo "A topic is required." >&2; exit 1; }

read -rp "Target name (e.g. \"CBC News\"): " name
[[ -z "$name" ]] && { echo "A target name is required." >&2; exit 1; }

read -rp "Target feed URL (RSS/Atom): " feed_url
[[ -z "$feed_url" ]] && { echo "A feed URL is required." >&2; exit 1; }

read -rp "Check interval [30m]: " check_interval
check_interval=${check_interval:-30m}

read -rp "Ingest URL [http://localhost:8765/proactive/ingest/news]: " ingest_url
ingest_url=${ingest_url:-http://localhost:8765/proactive/ingest/news}

slug=$(echo "$name" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+|-+$//g')
[[ -z "$slug" ]] && slug=target

config_path="$CONFIG_DIR/$slug.toml"
if [[ -f "$config_path" ]]; then
  read -rp "$config_path already exists - overwrite? [y/N]: " confirm
  [[ "$confirm" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 1; }
fi

mkdir -p "$CONFIG_DIR"
cat > "$config_path" <<EOF
name = "$name"
feed_url = "$feed_url"
check_interval = "$check_interval"
interests = ["$topic"]
ingest_url = "$ingest_url"
EOF
echo "Wrote $config_path"

echo "Building raven binary from $REPO_DIR..."
tmp_bin="$(mktemp)"
# -buildvcs=false: this runs under sudo, so the repo (owned by a regular user) looks like
# "dubious ownership" to git - Go's VCS stamping then fails the build entirely.
( cd "$REPO_DIR" && go build -buildvcs=false -o "$tmp_bin" . )
install -m 755 "$tmp_bin" "$BIN_PATH"
rm -f "$tmp_bin"
echo "Installed $BIN_PATH"

if [[ ! -f "$UNIT_PATH" ]]; then
  cp "$REPO_DIR/systemd/raven@.service" "$UNIT_PATH"
  echo "Installed $UNIT_PATH"
fi

systemctl daemon-reload
systemctl enable "raven@$slug.service"
systemctl restart "raven@$slug.service"

echo
echo "raven@$slug.service is running - check with: systemctl status raven@$slug"
