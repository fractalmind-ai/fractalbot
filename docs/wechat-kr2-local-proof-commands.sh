#!/usr/bin/env bash
set -euo pipefail

# O7 KR2 callback-first local proof helper
# This script prints the exact commands for the first runtime proof.
# It does not edit your real config automatically.

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG_OVERLAY="$REPO_ROOT/docs/wechat-local-config-overlay.example.yaml"
STATUS_URL="http://127.0.0.1:18789/status"
CALLBACK_URL="http://127.0.0.1:18810/wechat/callback"
TOKEN="local-test-token"
TIMESTAMP="1"
NONCE="2"
ECHOSTR="hello"

SIG=$(python3 - <<'PY2'
import hashlib
items = sorted(["local-test-token", "1", "2"])
print(hashlib.sha1("".join(items).encode()).hexdigest())
PY2
)

cat <<EOF
[1] Review local overlay:
  cat "$CONFIG_OVERLAY"

[2] Prepare a local config by copying the overlay fields into your private config.

[3] Start FractalBot with your local config:
  cd "$REPO_ROOT"
  fractalbot --config ./config.yaml

[4] Check gateway status:
  curl -s "$STATUS_URL" | jq '.'

[5] GET callback handshake proof:
  curl -i "$CALLBACK_URL?signature=$SIG&timestamp=$TIMESTAMP&nonce=$NONCE&echostr=$ECHOSTR"

[6] POST callback proof:
  curl -i     -X POST "$CALLBACK_URL?signature=$SIG&timestamp=$TIMESTAMP&nonce=$NONCE"     -H 'Content-Type: application/xml'     --data '<xml><ToUserName><![CDATA[toUser]]></ToUserName><FromUserName><![CDATA[fromUser]]></FromUserName><CreateTime>1348831860</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[this is a test]]></Content><MsgId>1234567890123456</MsgId></xml>'

[7] Record outputs into:
  docs/wechat-kr2-evidence-template.md
EOF
