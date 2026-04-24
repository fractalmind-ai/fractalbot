#!/usr/bin/env bash
set -euo pipefail

# O7 KR2 polling-first local proof helper
# This script prints the commands to collect the first polling proof.

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG_OVERLAY="$REPO_ROOT/docs/wechat-polling-config-overlay.example.yaml"
STATUS_URL="http://127.0.0.1:18789/status"
STATE_FILE="./workspace/wechat.polling.state.json"

cat <<EOF
[1] Review local polling overlay:
  cat "$CONFIG_OVERLAY"

[2] Merge overlay fields into your private local config.

[3] Start FractalBot with your local config:
  cd "$REPO_ROOT"
  fractalbot --config ./config.yaml

[4] Check gateway status:
  curl -s "$STATUS_URL" | jq '.'

[4.1] Extract WeChat polling runtime telemetry:
  curl -s "$STATUS_URL" | jq '.channels[] | select(.name=="wechat") | {mode, provider, polling}'

[5] Check polling state file:
  test -f "$STATE_FILE" && cat "$STATE_FILE" || echo "state file not created yet"

[6] Record outputs into:
  docs/wechat-kr2-polling-evidence-template.md
EOF
