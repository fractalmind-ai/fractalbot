# FractalBot CLI Usage Guide

This guide covers using the `fractalbot` CLI to send messages through various channels.

## Quick Start

```bash
# Send a message
fractalbot --config ~/.config/fractalbot/config.yaml message send \
  --channel telegram \
  --to "<recipient_id>" \
  --text "Hello from FractalBot"
```

## Installation

```bash
# macOS
brew install fractalbot

# Linux (from source)
go install github.com/fractalmind-ai/fractalbot/cmd/fractalbot@latest
```

## Configuration

By default, FractalBot looks for config at:
- macOS: `~/Library/Application Support/FractalBot/config.yaml`
- Linux: `${XDG_CONFIG_HOME:-$HOME/.config}/fractalbot/config.yaml`

Or specify explicitly with `--config /path/to/config.yaml`.

## Health Check

Check all channels at once via the gateway status endpoint:

```bash
curl -s http://127.0.0.1:18789/status | jq '.'
```

Example output:

```json
{
  "status": "ok",
  "channels": [
    {
      "name": "telegram",
      "enabled": true,
      "running": true,
      "last_error": "",
      "last_activity": "2026-03-05T14:00:00Z"
    },
    {
      "name": "imessage",
      "enabled": true,
      "running": true,
      "last_error": "",
      "last_activity": "2026-03-05T13:55:50Z"
    }
  ]
}
```

## Channel-Specific Usage

### Telegram

```bash
# Find your admin/allowed user IDs
grep -E "adminID|allowedUsers" ~/.config/fractalbot/config.yaml

# Send message
fractalbot --config ~/.config/fractalbot/config.yaml message send \
  --channel telegram \
  --to "1234567890" \
  --text "Hello from FractalBot"
```

### iMessage (macOS only)

```bash
# iMessage uses phone numbers or email addresses as recipients
fractalbot --config ~/.config/fractalbot/config.yaml message send \
  --channel imessage \
  --to "+8619575545051" \
  --text "Hello from FractalBot"
```

**Requirements:**
- Full Disk Access for FractalBot (System Settings → Privacy & Security → Full Disk Access)
- Messages app may need to be running for outbound messages

### Slack

```bash
# Slack recipient is the User ID (U12345678)
fractalbot --config ~/.config/fractalbot/config.yaml message send \
  --channel slack \
  --to "U12345678" \
  --text "Hello from FractalBot"
```

### Feishu/Lark

```bash
# Feishu recipient is the open_id
fractalbot --config ~/.config/fractalbot/config.yaml message send \
  --channel feishu \
  --to "ou_xxxxx" \
  --text "Hello from FractalBot"
```

### Discord

```bash
# Discord recipient is the User ID (snowflake)
fractalbot --config ~/.config/fractalbot/config.yaml message send \
  --channel discord \
  --to "123456789012345678" \
  --text "Hello from FractalBot"
```

## Service Management

### macOS (launchctl)

```bash
# Check if running
launchctl list | grep -i fractalbot

# Start service
launchctl kickstart -k gui/$(id -u)/ai.fractalmind.fractalbot

# View logs
log show --predicate 'process == "FractalBot"' --last 5m --info --debug
```

### Linux (systemd)

```bash
# Check status
systemctl --user status fractalbot

# Start service
systemctl --user start fractalbot

# View logs
journalctl --user -u fractalbot -f
```

## Troubleshooting

### Connection Refused

**Error:** `dial tcp 127.0.0.1:18789: connection refused`

**Cause:** FractalBot service is not running

**Fix:**
```bash
# macOS
launchctl kickstart -k gui/$(id -u)/ai.fractalmind.fractalbot

# Linux
systemctl --user start fractalbot
```

### Channel Not Found (404)

**Error:** `gateway API error (404): channel "telegram" not found`

**Cause:** Channel is disabled in config

**Fix:** Enable the channel in `~/.config/fractalbot/config.yaml`:
```yaml
channels:
  telegram:
    enabled: true
```

### Missing Recipient (400)

**Error:** `gateway API error (400): to is required`

**Cause:** Missing or invalid `--to` argument

**Fix:** Ensure you provide a valid recipient ID for the channel

### Downstream Error (502)

**Error:** `gateway API error (502): ...`

**Cause:** Channel-specific error (invalid token, network issues, permissions)

**Fix:** Check the channel configuration (bot token, permissions, network connectivity)

## Environment Variables

For convenience, set these in your shell profile:

```bash
# ~/.zshrc or ~/.bashrc
export FRACTALBOT_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/fractalbot/config.yaml"
export FRACTALBOT_HOST="127.0.0.1:18789"

# Alias for quick sends
alias fb='fractalbot --config "$FRACTALBOT_CONFIG" message send'
```

Usage:
```bash
fb --channel telegram --to "1234567890" --text "Hello"
```
