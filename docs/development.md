# Development

## Setup

```bash
git clone git@github.com:fractalmind-ai/fractalbot.git
cd fractalbot
go mod download
cp config.example.yaml config.yaml
```

Edit `config.yaml` to enable at least one channel. The complete option set and inline guidance live in [`config.example.yaml`](../config.example.yaml).

```bash
go run ./cmd/fractalbot --config ./config.yaml
```

## Local Telegram demo

For a polling-based Telegram-to-oh-my-code demo, set these values in `config.yaml`:

- `channels.telegram.botToken`
- `channels.telegram.adminID`
- `channels.telegram.allowedUsers`
- `agents.router: ohMyCode`
- `agents.ohMyCode.enabled: true`
- `agents.ohMyCode.workspace`
- `agents.ohMyCode.defaultAgent` and `allowedAgents`

Start the gateway:

```bash
go run ./cmd/fractalbot --config ./config.yaml
```

In a Telegram DM, use `/whoami` to confirm your user ID, `/ping` to verify the channel, then send a normal task or `/agent coder-a summarize current status`.

## Build and test

```bash
make build
make test
make vet
```

Equivalent focused commands:

```bash
go build -o fractalbot ./cmd/fractalbot
go test ./...
go test -v -race ./...
go vet ./...
```

Cross-platform builds are available through `make build-all`.

## WebSocket smoke test

Create a minimal config with no enabled channel:

```yaml
gateway:
  port: 18789
  bind: 127.0.0.1
channels:
  telegram:
    enabled: false
agents:
  workspace: ./workspace
  maxConcurrent: 1
```

Start the gateway in one terminal and the echo client in another:

```bash
go run ./cmd/fractalbot --config ./config.yaml
go run ./cmd/ws-echo-client --url ws://127.0.0.1:18789/ws
```

The client should receive a JSON `echo` event.

## Project structure

```text
fractalbot/
├── cmd/
│   ├── fractalbot/          # CLI and gateway entry point
│   └── ws-echo-client/      # WebSocket smoke-test client
├── internal/
│   ├── agent/               # Agent routers and desktop delivery
│   ├── bus/                 # In-process message bus
│   ├── channels/            # Channel adapters and workers
│   ├── config/              # YAML configuration
│   └── gateway/             # HTTP and WebSocket server
├── pkg/protocol/            # Shared protocol types
├── docs/                    # Operator and design documentation
├── config.example.yaml
└── Makefile
```

See [CONTRIBUTING.md](../CONTRIBUTING.md) for coding and pull request guidelines.
