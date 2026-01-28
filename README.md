# FractalBot

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%201.23-00ADD8E.svg)](https://golang.org)
[![Stars](https://img.shields.io/github/stars/fractalmind-ai/fractalbot?style=social)](https://github.com/fractalmind-ai/fractalbot/stargazers)

Multi-agent orchestration system for personal AI assistants - FractalMind-inspired architecture in Go.

## 📋 Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## 🎯 Overview

FractalBot is a Go-based reimagining of [Clawdbot](https://github.com/clawdbot/clawdbot) infused with FractalMind AI's philosophy of process-oriented multi-agent orchestration.

**Inspired by:**
- [Clawdbot](https://github.com/clawdbot/clawdbot) - Personal AI assistant gateway
- [FractalMind AI](https://github.com/fractalmind-ai) - Multi-agent team architectures

**Key Principles:**
- 🦞 **Local-first** - Run on your own devices, full control
- 🤖 **Multi-agent** - Coordinated agent teams with lead-based orchestration
- 🔄 **Process-oriented** - Strict workflows, quality gates, anti-drift
- 🌐 **Multi-channel** - Support for Telegram, Slack, Discord, and more
- 🔒 **Secure by default** - Explicit permissions, sandboxing, audit logs

## ✨ Features

- **Gateway Control Plane** - WebSocket-based session and channel management
- **Multi-Agent Runtime** - Parallel agent execution with team coordination
- **Channel Support** - Telegram, Slack, Discord (initially)
- **Tools Platform** - Browser control, file operations, system commands
- **Quality Gates** - Automated validation and quality checks
- **Memory System** - Persistent context and knowledge base

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
           ├─ Web Chat
           └─ Agent Runtime
                  │
                  ├─ Agent Manager (team orchestration)
                  ├─ Multiple Agent Sessions
                  └─ Tool Execution
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
# Clone the repository
git clone git@github.com:fractalmind-ai/fractalbot.git
cd fractalbot

# Build
go build -o fractalbot ./cmd/fractalbot

# Run (development mode)
./fractalbot gateway --port 18789
```

### Configuration

Create `config.yaml`:

```yaml
gateway:
  port: 18789
  bind: 127.0.0.1

channels:
  telegram:
    enabled: true
    botToken: "your_bot_token"
    adminID: 1234567890
    allowedUsers:
      - 1234567890

    # Optional: start a local webhook server
    webhookListenAddr: "0.0.0.0:18790"
    webhookPath: "/telegram/webhook"

    # Optional: register webhook with Telegram on startup
    webhookPublicURL: "https://your-domain.example/telegram/webhook"
    webhookSecretToken: "replace-with-random-secret"

agents:
  workspace: ./workspace
  maxConcurrent: 4
```

### Running the Gateway

```bash
./fractalbot gateway --port 18789 --verbose
```

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
│   └── fractalbot/          # Main entry point
├── internal/
│   ├── gateway/               # WebSocket gateway server
│   ├── agent/                # Agent runtime and manager
│   ├── channels/              # Channel implementations
│   ├── tools/                # Tool execution engine
│   └── config/               # Configuration management
├── pkg/
│   ├── types/                 # Shared types
│   └── protocol/              # Protocol definitions
├── web/                      # Web UI assets
├── scripts/                  # Build and utility scripts
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
