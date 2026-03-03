# FractalBot Roadmap

## Project Status

**Current Phase**: Phase 2.5 - Messaging UX + Routing Control Plane

FractalBot is now positioned as a pure messaging gateway. Scope is intentionally limited to channel integration, secure routing, and message ingress/egress APIs.

## Implementation Phases

### Phase 1: Core Gateway (Done)

- [x] WebSocket server implementation
- [x] Client connection handling
- [x] YAML configuration loading
- [x] Protocol message definitions
- [x] Base project structure

### Phase 2: Channel Integrations (In Progress)

#### Telegram
- [x] Polling/webhook lifecycle
- [x] Message routing and command handling
- [x] User/chat authorization
- [x] Message send API wiring

#### Feishu/Lark
- [x] Event connection
- [x] Message routing and command handling
- [x] User authorization

#### Slack (DM-only)
- [x] Socket Mode skeleton
- [x] Allowlist + onboarding commands
- [ ] Broader reliability hardening

#### Discord (DM-only)
- [x] Gateway skeleton
- [x] Allowlist + onboarding commands
- [ ] Broader reliability hardening

#### iMessage
- [x] Send + optional polling support
- [ ] Expanded operational coverage/testing

### Phase 2.5: Messaging UX & Routing Control Plane (In Progress)

#### Outbound Messaging
- [x] `POST /api/v1/message/send`
- [x] `fractalbot message send`
- [x] `fractalbot file download`
- [ ] Multi-target broadcast API/CLI
- [ ] Delivery receipts and retry policy

#### Routing + Policy
- [x] oh-my-code assign integration
- [x] Routing context injection (`channel/chat/user/agent/thread`)
- [ ] Conversation routing memory (origin channel/thread mapping)
- [ ] Per-agent outbound target policy
- [ ] Rate limiting + duplicate suppression

#### Operator Experience
- [x] `/status` and `/whoami` across channels
- [x] Agent lifecycle commands (`/monitor`, `/startagent`, `/stopagent`, `/doctor`)
- [ ] Cleaner operator diagnostics and error surfaces

## Engineering Backlog

### Testing
- [ ] Increase coverage for channel edge-cases and lifecycle commands
- [ ] Add integration tests per channel
- [ ] Add end-to-end tests for outbound message send/download

### Observability
- [ ] Structured logging
- [ ] Metrics endpoint
- [ ] Optional tracing hooks

### Security
- [ ] Rate limiting for inbound channel messages
- [ ] Audit trail for outbound delivery actions
- [ ] Secret rotation/reload strategy

## Success Criteria

- [ ] `go build ./...` and `go test ./...` remain green
- [ ] Telegram/Slack/Discord/Feishu/iMessage message paths are stable
- [ ] HTTP + CLI messaging surfaces are production-ready
- [ ] Default-deny authorization behavior is preserved across channels
