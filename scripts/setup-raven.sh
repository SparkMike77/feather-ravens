#!/usr/bin/env bash
# Installer for one Raven instance: takes a topic and a target feed - via flags, or interactively
# for anything a flag doesn't cover - writes /etc/feather-ravens/<slug>.toml, (re)builds and
# installs the binary, installs the raven@.service unit if it isn't there yet, and enables+starts
# raven@<slug>. Requires sudo - it writes under /etc and /usr/local/bin and manages a systemd
# unit.
#
# This is the one place that logic lives. web/index.html's command builder generates a call to
# this script rather than re-implementing its steps, and so does Feather's own stage_raven_config
# tool (feather/proactive/raven_config.py, in the feather-1 repo) - three different ways to reach
# this same install path used to exist and had quietly drifted out of sync with each other.
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: setup-raven.sh [options]

  --topic TEXT        Topic(s) to watch for, comma-separated, e.g. "AI" or "AI,climate"
  --name TEXT          Target name, e.g. "CBC News"
  --feed-url URL        Target RSS/Atom feed URL
  --interval DURATION   Check interval, e.g. "30m" (default: 30m)
  --ingest-url URL      Where matches get posted (default: http://localhost:8765/proactive/ingest/news)
  -y, --yes             Don't prompt before overwriting an existing config for this target
  -h, --help             Show this help

Any option left out is prompted for interactively.
USAGE
}

topic=""
name=""
feed_url=""
check_interval=""
ingest_url=""
assume_yes=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --topic) topic="$2"; shift 2 ;;
    --name) name="$2"; shift 2 ;;
    --feed-url) feed_url="$2"; shift 2 ;;
    --interval) check_interval="$2"; shift 2 ;;
    --ingest-url) ingest_url="$2"; shift 2 ;;
    -y|--yes) assume_yes=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "Run this with sudo - it writes to /etc/feather-ravens, /usr/local/bin, and manages systemd units." >&2
  exit 1
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_DIR=/etc/feather-ravens
BIN_PATH=/usr/local/bin/raven
UNIT_PATH=/etc/systemd/system/raven@.service

[[ -z "$topic" ]] && read -rp "Topic(s) to watch for, comma-separated (e.g. \"AI\" or \"AI,climate\"): " topic
[[ -z "$topic" ]] && { echo "A topic is required." >&2; exit 1; }

[[ -z "$name" ]] && read -rp "Target name (e.g. \"CBC News\"): " name
[[ -z "$name" ]] && { echo "A target name is required." >&2; exit 1; }

[[ -z "$feed_url" ]] && read -rp "Target feed URL (RSS/Atom): " feed_url
[[ -z "$feed_url" ]] && { echo "A feed URL is required." >&2; exit 1; }

if [[ -z "$check_interval" ]]; then
  read -rp "Check interval [30m]: " check_interval
  check_interval=${check_interval:-30m}
fi

if [[ -z "$ingest_url" ]]; then
  read -rp "Ingest URL [http://localhost:8765/proactive/ingest/news]: " ingest_url
  ingest_url=${ingest_url:-http://localhost:8765/proactive/ingest/news}
fi

slug=$(echo "$name" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+|-+$//g')
[[ -z "$slug" ]] && slug=target

# "AI" or "AI, climate" -> ["AI"] or ["AI", "climate"] - matches Feather's own raven.toml
# generation (feather/proactive/raven_config.py), which can carry more than one explicit interest.
IFS=',' read -ra topic_arr <<< "$topic"
interests_toml=""
for t in "${topic_arr[@]}"; do
  t_trimmed="$(echo "$t" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  [[ -z "$t_trimmed" ]] && continue
  [[ -n "$interests_toml" ]] && interests_toml+=", "
  interests_toml+="\"$t_trimmed\""
done
[[ -z "$interests_toml" ]] && { echo "At least one non-empty topic is required." >&2; exit 1; }

config_path="$CONFIG_DIR/$slug.toml"
if [[ -f "$config_path" && "$assume_yes" -ne 1 ]]; then
  read -rp "$config_path already exists - overwrite? [y/N]: " confirm
  [[ "$confirm" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 1; }
fi

mkdir -p "$CONFIG_DIR"
cat > "$config_path" <<EOF
name = "$name"
feed_url = "$feed_url"
check_interval = "$check_interval"
interests = [$interests_toml]
ingest_url = "$ingest_url"
EOF
echo "Wrote $config_path"

echo "Building raven binary from $REPO_DIR..."
# -buildvcs=false: this runs under sudo, so the repo (owned by a regular user) looks like
# "dubious ownership" to git - Go's VCS stamping then fails the build entirely.
# -o writes straight to BIN_PATH - confirmed safe against a currently-running raven@<slug>
# (Go builds to a temp file and replaces the destination, not an in-place write): a plain `cp`
# onto an already-running executable fails with "Text file busy" instead, which is what a manual
# rebuild-in-place used to hit here.
( cd "$REPO_DIR" && go build -buildvcs=false -o "$BIN_PATH" . )
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
