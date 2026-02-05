# Slack Setup (Socket Mode)

FractalBot uses Slack Socket Mode (no public ports) and DM-only message handling. This doc covers the minimal setup.

## 1) Create a Slack App + Enable Socket Mode

1. Create a new Slack app in your workspace.
2. Enable **Socket Mode**.
3. Create an **App-Level Token** with scope: `connections:write` (token starts with `xapp-`).

## 2) Bot Token + OAuth Scopes

Add a bot user, then install the app to your workspace to obtain a **Bot Token** (starts with `xoxb-`).

Minimal scopes for DM support:
- `chat:write` (send replies)
- `im:history` (receive DM messages)

## 3) Event Subscriptions

Subscribe to **Bot Events**:
- `message.im` (DM messages)

## 4) Configure FractalBot

```yaml
channels:
  slack:
    enabled: true
    botToken: "xoxb-your-bot-token"
    appToken: "xapp-your-app-level-token"
    allowedUsers:
      - "U12345678"  # your Slack user ID
```

Notes:
- DM-only: messages in channels are ignored.
- Allowlist: only `allowedUsers` can run commands. Safe commands like `/whoami`, `/status`, and `/agents` are permitted for onboarding.

## 5) Verify

In a Slack DM with the bot:

- `/whoami` → shows your Slack IDs (add to `allowedUsers` if needed)
- `/status` → basic bot status (no secrets)
- `/agents` → list allowed agents or config hints
- `/tools` → runtime tools list (if runtime enabled; otherwise shows disabled message)

## Safety

- Socket Mode means no public HTTP endpoints.
- DM-only + allowlist prevent unintended exposure.
