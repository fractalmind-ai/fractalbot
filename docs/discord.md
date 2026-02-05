# Discord Setup (DM-Only)

FractalBot uses Discord Gateway events for DM-only message handling. This doc covers the minimal setup.

## 1) Create App + Bot Token

1. Create a new Discord application in the Developer Portal.
2. Add a **Bot** and copy the **Bot Token**.

## 2) Enable Intents

In **Bot → Privileged Gateway Intents**:
- Enable **Message Content Intent** (required to read DM text).

## 3) Invite (Optional)

DMs require the user to share a server with the bot or be friends. If you need to add the bot to a server,
use the OAuth2 URL Generator:
- Scopes: `bot`
- Permissions: none (0)

## 4) Configure FractalBot

```yaml
channels:
  discord:
    enabled: true
    token: "your-bot-token"
    allowedUsers:
      - "123456789012345678" # your Discord user ID
```

Notes:
- DM-only: messages in servers are ignored.
- Allowlist: only `allowedUsers` can run commands. Safe commands like `/whoami`, `/status`, and `/agents` are permitted for onboarding.
- `/agent <name> <task>` requires allowlist approval.

## 5) Verify

In a Discord DM with the bot:

- `/whoami` → shows your Discord IDs (add to `allowedUsers` if needed)
- `/status` → basic bot status (no secrets)
- `/agents` → list allowed agents or config hints
- `/tools` → runtime tools list (if runtime enabled; otherwise shows disabled message)

## Safety

- DM-only + allowlist prevent unintended exposure.
- No public ports are required.
