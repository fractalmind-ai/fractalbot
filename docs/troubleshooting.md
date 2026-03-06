# Troubleshooting: Failure Handling and Rollback

This guide is based on FractalBot's current runtime and error-handling code in:

- `cmd/fractalbot/main.go`
- `internal/config/config.go`
- `internal/gateway/server.go`
- `internal/channels/*`
- `internal/agent/manager.go`

If you follow the steps below, you can usually recover without data loss.

## 0) Fast Triage (Run First)

### Symptoms

- Bot is silent
- `fractalbot message send` fails
- Gateway APIs are unreachable

### Likely Cause

- Gateway process is down, wrong bind/port, or one channel failed to start.

### Fix

```bash
# 1) Basic gateway health
curl -sS http://127.0.0.1:18789/health

# 2) Full runtime status (channels + last_error timestamps)
curl -sS http://127.0.0.1:18789/status | python3 -m json.tool

# 3) CLI path check (goes through /api/v1/message/send)
fractalbot --config ./config.yaml message send \
  --channel telegram \
  --to 123456789 \
  --text "health check"
```

### Rollback and Recovery

```bash
# If you run as systemd user service
systemctl --user restart fractalbot.service
journalctl --user -u fractalbot.service -n 200 --no-pager
```

---

## 1) Config Errors (YAML, Required Fields, Invalid Agent Settings)

### Symptoms

Common startup errors:

- `failed to load config: failed to read config: ...`
- `failed to load config: failed to parse config: ...`
- `channels.telegram.botToken is required when telegram is enabled`
- `channels.slack.botToken and channels.slack.appToken are required when slack is enabled`
- `channels.discord.token is required when discord is enabled`
- `channels.imessage.recipient is required when imessage is enabled`
- `agents.ohMyCode.defaultAgent: must be in agents.ohMyCode.allowedAgents`
- `agents.ohMyCode.agentManagerScript: must be within agents.ohMyCode.workspace`

### Likely Cause

- Invalid YAML or missing required fields when a channel is enabled.
- `agents.ohMyCode` validation failure (invalid agent name, allowlist mismatch, workspace/script path issues).

### Fix

```bash
# Keep a backup before editing
cp config.yaml config.yaml.bak.$(date +%Y%m%d%H%M%S)

# Start from known-good schema
cp config.example.yaml /tmp/config.example.yaml

# Open and fix required fields for enabled channels
# telegram: botToken
# slack: botToken + appToken
# discord: token
# imessage: recipient
# ohMyCode: workspace + valid defaultAgent/allowedAgents
```

Quick checks:

```bash
# Verify oh-my-code script path is inside workspace
# (matches internal/config validation logic)
python3 - <<'PY'
import os
workspace = "/path/to/oh-my-code"
script = ".claude/skills/agent-manager/scripts/main.py"
resolved = os.path.realpath(os.path.join(workspace, script))
print("OK" if resolved.startswith(os.path.realpath(workspace)+os.sep) else "BAD", resolved)
PY
```

### Rollback and Recovery

```bash
# Roll back to last known-good config
cp config.yaml.bak.<timestamp> config.yaml

# Restart gateway
systemctl --user restart fractalbot.service || true
# or
fractalbot --config ./config.yaml
```

---

## 2) Token/Credential Invalid or Revoked

### Symptoms

Examples from runtime logs/errors:

- Telegram polling: `Telegram polling error: telegram getUpdates failed: ...`
- Telegram webhook registration: `telegram setWebhook failed: ...`
- Slack runtime: `slack socket mode error: ...`
- Feishu send: `feishu send failed: code=<code> msg=<msg>`
- Discord init/open fails with token/auth errors from Discord API

### Likely Cause

- Expired/revoked/incorrect token (`botToken`, `appToken`, `token`, `appSecret`).

### Fix

1. Rotate token in provider console (BotFather / Slack App / Discord Developer Portal / Feishu/Lark console).
2. Update `config.yaml`.
3. Restart FractalBot.
4. Re-check:

```bash
curl -sS http://127.0.0.1:18789/status | python3 -m json.tool
```

### Rollback and Recovery

- Temporarily disable only the broken channel (`enabled: false`) and keep others running.
- Restore previous token from secure secret manager if rotation was accidental.

---

## 3) Network Issues (Gateway Timeout, Webhook Callback Failure)

### Symptoms

- CLI send path:
  - `failed to send message: request http://127.0.0.1:18789/api/v1/message/send failed: ...`
- Telegram webhook:
  - HTTP `401` from webhook endpoint when secret header mismatch
  - `Telegram webhook server error: ...`
  - `Failed to parse Telegram webhook update: ...`

### Likely Cause

- Gateway not listening on expected bind/port.
- Firewall/reverse proxy/public URL mismatch.
- Telegram webhook secret token mismatch.

### Fix

```bash
# Check local listener
ss -ltnp | rg 18789 || lsof -i :18789

# Check gateway endpoints
curl -sS http://127.0.0.1:18789/health
curl -sS http://127.0.0.1:18789/status | python3 -m json.tool
```

For webhook mode:

- Ensure `webhookPublicURL` is public HTTPS.
- Ensure reverse proxy forwards `X-Telegram-Bot-Api-Secret-Token` unchanged.
- Ensure `webhookPath` matches exactly.

### Rollback and Recovery

If webhook path is unstable, roll back to polling mode (local-friendly):

```yaml
channels:
  telegram:
    mode: "polling"
    webhookRegisterOnStart: false
    webhookDeleteOnStop: false
    webhookListenAddr: ""
    webhookPublicURL: ""
```

Then restart service.

---

## 4) Message Send Failures (`channel_not_found`, rate limit, channel not enabled)

### Symptoms

From `fractalbot message send` / gateway API:

- `gateway API error (404): channel "slack" not found`
- `gateway API error (502): ...`
- Slack API may bubble errors such as `channel_not_found` or `ratelimited`
- Telegram send path may return `telegram API returned status <code>: ...`

### Likely Cause

- Channel is not enabled/registered.
- Wrong `--channel` or wrong target ID (`--to`).
- Provider-side constraints (invalid channel, rate limit, permission scope).

### Fix

```bash
# Confirm channel is enabled and running
curl -sS http://127.0.0.1:18789/status | python3 -m json.tool

# Retry with a known-good target
fractalbot --config ./config.yaml message send \
  --channel slack \
  --to C0123456789 \
  --text "retry after verification"
```

If using Slack thread replies, verify `--thread-ts` is valid for that channel.

### Rollback and Recovery

- Remove `--thread-ts` and send as a normal message first.
- Route urgent outbound notification through another healthy channel until Slack/Telegram target is fixed.

---

## 5) Gateway Crash/Restart and State Recovery

### Symptoms

- `/health` unreachable
- Process exits and no new channel events
- systemd repeatedly restarts service

### Likely Cause

- Fatal startup error (config/channel init failure).
- Runtime crash in environment/dependency.

### Fix

```bash
# systemd user service status/logs
systemctl --user status fractalbot.service --no-pager
journalctl --user -u fractalbot.service -n 200 -f

# Manual foreground run for immediate stack/log visibility
fractalbot --config ./config.yaml --verbose
```

### Rollback and Recovery

```bash
# Revert to last known-good config
cp config.yaml.bak.<timestamp> config.yaml

# Restart service
systemctl --user restart fractalbot.service
```

Telegram polling recovery note:

- `pollingOffsetFile` persists `UpdateID+1`.
- Keep this file writable to avoid duplicate update processing after restart.
- If offset file is corrupted, fix/remove it and restart:

```bash
rm -f ./workspace/telegram.offset
systemctl --user restart fractalbot.service
```

---

## 6) Agent Integration Failures (timeout, assign/start/stop/monitor errors)

### Symptoms

User-facing messages:

- `❌ agent-manager error; please check server logs`
- `❌ Default agent is missing or invalid...`
- `❌ agent "..." is not allowed`

Server logs/errors:

- `oh-my-code agent-manager failed: ...`
- `agents.ohMyCode is disabled`
- `agents.ohMyCode.workspace is required`
- assign timeout path via `assignTimeoutSeconds` / context deadline

### Likely Cause

- `agents.ohMyCode.enabled` is false or workspace/script path is wrong.
- Agent name not in allowlist.
- Agent manager process is unhealthy or too slow for current timeout.

### Fix

```bash
# In your oh-my-code workspace
python3 .claude/skills/agent-manager/scripts/main.py doctor
python3 .claude/skills/agent-manager/scripts/main.py list
python3 .claude/skills/agent-manager/scripts/main.py start qa-1
python3 .claude/skills/agent-manager/scripts/main.py monitor qa-1 --lines 120
```

And in `config.yaml`:

- `agents.ohMyCode.enabled: true`
- `agents.ohMyCode.workspace: /absolute/path/to/oh-my-code`
- `defaultAgent` exists and is allowlisted when `allowedAgents` is set
- increase `assignTimeoutSeconds` if jobs are slow

### Rollback and Recovery

- Temporarily set `agents.ohMyCode.enabled: false` to keep gateway online (FractalBot falls back to echo behavior for inbound tasks).
- Re-enable after `doctor` and per-agent checks pass.

---

## 7) iMessage-Specific Failures (macOS permissions/runtime)

### Symptoms

- `imessage channel is only supported on darwin`
- `imessage database not accessible (...)`
- `imessage database permission check failed: ...`
- `imessage app permission check failed: ...`
- `messages app did not start in time`
- `imessage osascript failed: ...`

### Likely Cause

- Running on non-macOS host.
- Missing Full Disk Access / Automation permissions for `sqlite3` or `osascript`.
- Messages app not started or inaccessible.

### Fix

1. Run iMessage channel only on macOS.
2. Grant required permissions (Terminal + Messages automation / database access).
3. Verify recipient + service (`E:iMessage` by default).

### Rollback and Recovery

- Disable `channels.imessage.enabled` and keep other channels running.
- Re-enable after permission checks pass.

---

## Operational Safety Tips

- Always backup `config.yaml` before edits.
- Change one channel at a time, then validate with `/status`.
- For production-like usage, prefer systemd user service + log tailing.
- Keep webhook mode only when HTTPS ingress is stable; otherwise use polling.
