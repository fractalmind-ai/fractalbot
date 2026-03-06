# FractalBot

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%201.23-00ADD8E.svg)](https://golang.org)
[![Stars](https://img.shields.io/github/stars/fractalmind-ai/fractalbot?style=social)](https://github.com/fractalmind-ai/fractalbot/stargazers)

Pure CLI + HTTP messaging gateway for routing channel messages to external agents.

## 📋 Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Troubleshooting](#-troubleshooting)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## 🎯 Overview

FractalBot is a Go-based messaging gateway inspired by [Clawdbot](https://github.com/clawdbot/clawdbot), focused on reliable channel I/O and external agent routing.

**Inspired by:**
- [Clawdbot](https://github.com/clawdbot/clawdbot) - Personal AI assistant gateway
- [FractalMind AI](https://github.com/fractalmind-ai) - Multi-agent team architectures

**Key Principles:**
- 🦞 **Local-first** - Run on your own devices, full control
- 🌐 **Multi-channel** - Telegram, Slack, Discord, Feishu, iMessage
- 🔌 **Gateway-first** - Clean message ingress/egress via HTTP + CLI + WebSocket
- 🧭 **External-agent routing** - Route tasks to oh-my-code agent-manager
- 🔒 **Secure by default** - Channel user allowlists with deny-by-default behavior

## ✨ Features

- **Gateway Control Plane** - WebSocket + HTTP server for messaging and status
- **Multi-Channel Support** - Telegram, Slack, Discord, Feishu/Lark, iMessage
- **Message Send API** - `POST /api/v1/message/send` + `fractalbot message send`
- **File Download CLI/API** - Pull channel attachments via gateway auth context
- **oh-my-code Routing** - Assign workflow with routing context injection
- **Security Model** - Per-channel allowlists and strict default deny

## 🏗️ Architecture

```
┌─────────────────────────────────────────┐
│         FractalBot Gateway          │
│      (Go WebSocket Server)           │
└──────────┬──────────────────────────┘
           │
           ├─ Telegram Bot
           ├─ Slack Bot
           ├─ Discord Bot
           ├─ Feishu/Lark Bot
           ├─ iMessage Bridge
           └─ External Agent Manager
```

**Inspired by Clawdbot's architecture but reimagined with:**
- Go's concurrency model for parallel agent execution
- FractalMind's process-oriented workflows
- Enhanced multi-agent coordination

## 🚀 Quick Start

### Prerequisites

- Go 1.23 or higher
- A Telegram Bot Token (for Telegram channel)
- Or Slack/Discord tokens (for other channels)

### Installation

```bash
# One-line install (pinned; builds and installs to ~/.local/bin)
curl -fsSL https://raw.githubusercontent.com/fractalmind-ai/fractalbot/cb052356b79b4e679efb03a93210ab4628590076/install.sh | \
  FRACTALBOT_REF=cb052356b79b4e679efb03a93210ab4628590076 bash

# Optional: install as a systemd user service (Linux)
curl -fsSL https://raw.githubusercontent.com/fractalmind-ai/fractalbot/cb052356b79b4e679efb03a93210ab4628590076/install.sh | \
  FRACTALBOT_REF=cb052356b79b4e679efb03a93210ab4628590076 bash -s -- --systemd-user

# Smoke check
~/.local/bin/fractalbot --help

# Clone the repository
git clone git@github.com:fractalmind-ai/fractalbot.git
cd fractalbot

# Build
go build -o fractalbot ./cmd/fractalbot

# Run (development mode)
./fractalbot --config ./config.yaml
```

### Configuration

Create `config.yaml`:

```yaml
gateway:
  port: 18789
  bind: 127.0.0.1
  # Optional: restrict allowed WebSocket origins
  # allowedOrigins:
  #   - "http://localhost:3000"

channels:
  telegram:
    enabled: true
    # DM-only; non-private chats are ignored.
    botToken: "your_bot_token"
    adminID: 1234567890
    allowedUsers:
      - 1234567890
    # Optional: restrict to specific chat IDs
    # allowedChats:
    #   - 1234567890

    # Recommended: long polling (local/dev friendly)
    mode: "polling"
    pollingTimeoutSeconds: 25
    pollingLimit: 100
    pollingOffsetFile: "./workspace/telegram.offset"

    # Optional: webhook mode (public HTTPS required)
    # mode: "webhook"
    # webhookListenAddr: "0.0.0.0:18790"
    # webhookPath: "/telegram/webhook"
    # webhookPublicURL: "https://your-domain.example/telegram/webhook"
    # webhookSecretToken: "replace-with-random-secret"
    # webhookRegisterOnStart: true
    # webhookDeleteOnStop: true

agents:
  workspace: ./workspace
  maxConcurrent: 4

  # Optional: route Telegram messages to oh-my-code agent-manager
  # (requires python3 + tmux + agent-manager installed in that workspace)
  ohMyCode:
    enabled: false
    workspace: "/home/elliot245/workspace/elliot245/oh-my-code"
    defaultAgent: "qa-1"
    allowedAgents:
      - "qa-1"
      - "coder-a"
    # FractalBot injects inbound routing context into assign prompts:
    # channel/chat_id/user_id/username + resolved selected_agent.
    # For Telegram outbound intent, the prompt directs the agent to
    # prefer use-fractalbot and default recipient to current chat_id if omitted.
    # Keep skill path/name consistent in the agent workspace:
    # .claude/skills/use-fractalbot/SKILL.md
```

### Routing Model

Telegram supports `/agent <name> <task...>` to route tasks to a specific agent; if omitted, `defaultAgent` is used. When `allowedAgents` is set, only those names are accepted. Use `/agents` to see allowed agents; if you target a disallowed agent, the bot will suggest `/agents`.
For Telegram-routed assignments, FractalBot includes routing context (`channel`, `chat_id`, `user_id`, `username`, `selected_agent`) in the assign prompt and explicitly hints outbound-send intent through `use-fractalbot`. If no Telegram recipient is provided, the prompt contract defaults target to current `chat_id`.
Operator note: make sure `use-fractalbot` appears in the agent's effective available skills list and points to `.claude/skills/use-fractalbot/SKILL.md`.
`/tool` and `/tools` are intentionally unavailable in gateway mode.

Additional lifecycle commands:
- `/agents` (list allowed agent names)
- `/monitor <name> [lines]` (show recent agent output; lines capped to 200)
- `/startagent <name>` (admin only)
- `/stopagent <name>` (admin only)
- `/doctor` (admin only)
- `/whoami` (show your Telegram IDs)
- `/ping` (simple health check)

Slack (DM-only) uses Socket Mode (no public ports) and requires allowlisting user IDs before messages are processed:

```yaml
channels:
  slack:
    enabled: true
    botToken: "xoxb-your-bot-token"
    appToken: "xapp-your-app-token"
    allowedUsers:
      - "U12345678"
```

Discord (DM-only) uses the gateway websocket (no public ports) and requires allowlisting user IDs before messages are processed:

```yaml
channels:
  discord:
    enabled: true
    token: "your-bot-token"
    allowedUsers:
      - "123456789012345678"
```

Tip: `/whoami` works in Slack/Discord DMs even before allowlisting, so users can self-identify.

### Local Demo (Telegram + polling + oh-my-code)

This is the fastest path to a local, end-to-end demo after the CLI entrypoint lands.

1) Copy the example config:

```bash
cp config.example.yaml config.yaml
```

2) Edit the minimum fields:

- `channels.telegram.botToken`
- `channels.telegram.adminID`
- `channels.telegram.allowedUsers` (include your Telegram user ID)
- `agents.ohMyCode.enabled: true`
- `agents.ohMyCode.workspace` (path to your oh-my-code repo)
- `agents.ohMyCode.defaultAgent` (e.g. `qa-1` or `coder-a`)
- `agents.ohMyCode.allowedAgents` (include default agent and any others you want)

3) Start the gateway:

```bash
# Option A: build + run
make build
./fractalbot --config ./config.yaml

# Option B: go run (config default is ./config.yaml)
go run ./cmd/fractalbot --config ./config.yaml
```

4) Start the gateway and fetch your Telegram IDs:

- Message: `/whoami`
- Expected: reply includes your User ID. Add it to `channels.telegram.allowedUsers`, restart the bot, and try again.

5) Send Telegram messages:

- Health check:
  - Message: `/ping`
  - Expected: `pong`

- Normal routing:
  - Message: `Hello from demo`
  - Expected: bot replies with agent-manager output (assignment confirmation plus recent monitor snapshot).
- Explicit agent selection:
  - Message: `/agent coder-a summarize current status`
  - Expected: task is routed to `coder-a`. If not allowed, the bot replies with an `agent \"...\" is not allowed` error.

If the bot is silent, verify the allowlist, and confirm `agents.ohMyCode.enabled: true` and that the agent-manager is running in the oh-my-code workspace.

### Running the Gateway

```bash
./fractalbot --config ./config.yaml --verbose
```

## 🩺 Troubleshooting

If you hit startup/runtime issues (config validation, token revocation, webhook failures, channel send errors, systemd restart loops, or agent-manager timeouts), use:

- [Troubleshooting: Failure Handling and Rollback](docs/troubleshooting.md)

### Smoke Test (WebSocket Echo)

```bash
# Minimal config for echo (no Telegram token required)
cat > config.yaml <<'EOF'
gateway:
  port: 18789
  bind: 127.0.0.1
channels:
  telegram:
    enabled: false
agents:
  workspace: ./workspace
  maxConcurrent: 1
EOF

# Terminal 1: start the gateway
go run ./cmd/fractalbot --config config.yaml

# Terminal 2: send an echo event
go run ./cmd/ws-echo-client --url ws://127.0.0.1:18789/ws
```

Expected output (JSON echo):

```json
{"kind":"event","action":"echo","data":{"text":"hello"}}
```

## 🔧 Development

### Project Structure

```
fractalbot/
├── cmd/
│   ├── fractalbot/            # Main entry point
│   └── ws-echo-client/        # Local WS smoke-test helper
├── internal/
│   ├── gateway/               # WebSocket gateway server
│   ├── agent/                 # oh-my-code routing manager
│   ├── channels/              # Channel implementations
│   └── config/               # Configuration management
├── pkg/
│   └── protocol/              # Protocol definitions
├── docs/                      # Channel/setup docs
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```

### Building

```bash
# Build for current platform
go build -o fractalbot ./cmd/fractalbot

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o fractalbot-linux ./cmd/fractalbot
GOOS=darwin GOARCH=amd64 go build -o fractalbot-mac ./cmd/fractalbot
GOOS=windows GOARCH=amd64 go build -o fractalbot.exe ./cmd/fractalbot
```

### Running Tests

```bash
go test ./...
go test -v -race ./...
```

## 🤝 Contributing

Contributions are welcome! Please read our [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🔗 Links

- [FractalMind AI](https://github.com/fractalmind-ai) - Organization
- [Clawdbot](https://github.com/clawdbot/clawdbot) - Inspiration source
- [Agent Manager](https://github.com/fractalmind-ai/agent-manager-skill)
- [Team Manager](https://github.com/fractalmind-ai/team-manager-skill)

## 🎮 Acknowledgments

- Inspired by [Clawdbot](https://github.com/clawdbot/clawdbot) by Peter Steinberger
- Process-oriented workflows from [FractalMind AI](https://github.com/fractalmind-ai)
- Community contributors and feedback
