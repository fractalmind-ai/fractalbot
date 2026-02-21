# FractalBot Roadmap

## 🎯 Project Status

**Current Phase**: Phase 2.5 - Messaging UX + Routing Control Plane

**Progress**: Core gateway complete. Telegram + Feishu/Lark channels are production-ready; Slack/Discord DM-only skeletons landed. Agent runtime skeleton started.

**Roadmap Adjustment (2026-02-09)**: Prioritize user-facing messaging UX and channel routing control before deeper agent-runtime expansion. This aligns implementation order with real-world usage quality (clean replies, proactive messaging, cross-channel continuity).

---

## 📋 Implementation Phases

### Phase 1: Core Gateway ✅ (DONE)

- [x] WebSocket server implementation
- [x] Client connection handling
- [x] Configuration system (YAML)
- [x] Protocol message definitions
- [x] Basic project structure

**Status**: Complete

---

### Phase 2: Channel Integrations 🚧 (IN PROGRESS)

#### Telegram Channel
- [x] Bot initialization (polling/webhook)
- [x] Message handling and routing
- [x] User authorization and validation
- [x] Command parsing and execution
- [x] Message sending to Telegram

#### Feishu/Lark Channel
- [x] Long-connection events (websocket)
- [x] Message handling + routing
- [x] User authorization and validation
- [x] Command parsing (/help, /whoami, /agents)

**Priority**: High
**Estimated**: Complete

#### Slack Channel
- [x] DM-only skeleton + allowlist + /whoami onboarding
- [ ] Bot initialization and RTM connection
- [ ] Event handling (message, mention, reaction)
- [ ] Team/channel authorization
- [ ] Slash command support
- [ ] Message posting to Slack

**Priority**: Medium
**Estimated**: 2-3 days

#### Discord Channel
- [x] DM-only skeleton + allowlist + /whoami onboarding
- [ ] Bot initialization and gateway connection
- [ ] Event handling (message, interaction)
- [ ] Guild/channel authorization
- [ ] Slash command support
- [ ] Message sending to Discord

**Priority**: Medium
**Estimated**: 2-3 days

---

### Phase 2.5: Messaging UX & Routing Control Plane 🆕 (IN PROGRESS)

#### Conversational Reply Experience
- [x] Default-agent conversational mode for plain messages (no raw monitor dump)
- [x] Direct assign acknowledgement uses concise `"处理中…"` reply (#277)
- [ ] Separate user-facing replies from operator diagnostics (`/monitor`, `/doctor`)
- [ ] Reply normalization (silent/heartbeat filtering + concise fallback behavior)
- [ ] Friendly progress/error responses (actionable next-step hints)

**Priority**: Critical
**Estimated**: 1-2 weeks

#### Outbound Messaging API + CLI
- [x] Unified outbound message API (single-target send: `POST /api/v1/message/send`) (#275)
- [x] CLI surface for send (`fractalbot message send`) (#276)
- [ ] CLI/API surface for proactive notify
- [ ] CLI/API surface for multi-target broadcast
- [ ] Default "reply to originating channel" behavior when target is omitted
- [ ] Multi-target fan-out (broadcast) with delivery receipts

**Priority**: Critical
**Estimated**: 1-2 weeks

#### Routing, Policy, and Safety
- [ ] Conversation routing memory (origin channel/thread/account mapping)
- [ ] Per-agent allowed channels/targets policy enforcement
- [ ] Rate limiting + duplicate suppression for proactive/broadcast sends
- [ ] Audit trail for outbound delivery actions

**Priority**: High
**Estimated**: 1 week

---

### Phase 3: Agent Runtime 🔜 (PLANNED)

#### Agent Manager
- [ ] Agent lifecycle management (start/stop/monitor)
- [ ] Session isolation and context management
- [ ] Agent-to-agent communication
- [ ] Memory and knowledge base integration
- [ ] Parallel execution with goroutines

**Priority**: High
**Estimated**: 2-3 weeks

#### Tool Execution Engine
- [x] Tool registry skeleton (default-deny allowlist)
- [x] Tool execution with sandboxing
- [x] File operations (read/write/edit/delete/list/tail/exists/stat/sha256/grep)
- [x] System command execution
- [ ] Browser control integration
- [ ] Canvas integration
  - Note: browser.canvas exists as a stub and is not wired yet.

**Priority**: High
**Estimated**: 2-3 weeks

---

### Phase 4: Multi-Agent Orchestration 📋 (PLANNED)

#### Team Management
- [ ] Team definition and configuration
- [ ] Lead-based coordination
- [ ] Task decomposition and assignment
- [ ] Progress tracking and monitoring
- [ ] Quality gate execution

**Priority**: Medium
**Estimated**: 5-7 days

#### Workflow Engine
- [ ] GitHub Issues workflow
- [ ] Process-oriented templates
- [ ] Anti-drift mechanisms
- [ ] Evidence-based reporting
- [ ] Output contract enforcement

**Priority**: Medium
**Estimated**: 5-7 days

---

### Phase 5: Web UI & Control Surface 🌐 (PLANNED)

- [ ] Web-based dashboard
- [ ] Real-time agent status display
- [ ] Channel management interface
- [ ] Configuration editor
- [ ] Log viewer and export

**Priority**: Low
**Estimated**: 7-10 days

---

## 🔧 Technical Debt & Improvements

### Testing (Current: 6/10 → Target: 9/10)
- [ ] Add comprehensive unit tests (target: 80% coverage)
- [ ] Integrate coverage reporting into CI (`go test -coverprofile`, upload to Codecov/Coveralls)
- [ ] Add integration tests for each channel (Telegram, Slack, Discord, Feishu end-to-end flows)
- [ ] Add end-to-end messaging UX tests (plain chat, proactive notify, broadcast)
- [ ] Add concurrency/stress tests (parallel message handling, connection storms, agent contention)
- [ ] Add fuzz testing for protocol message parsing and config loading
- [ ] Add benchmark tests for hot paths (message routing, tool dispatch, sandbox validation)

### Observability & Reliability (Current: 6/10 → Target: 9/10)
- [ ] Replace `log` with structured logging (zerolog or zap) — JSON output, log levels, request tracing
- [ ] Add OpenTelemetry tracing (gateway → channel → agent → tool spans)
- [ ] Add Prometheus metrics (message latency, agent utilization, tool execution duration, error rates)
- [ ] Expose `/metrics` endpoint for scraping
- [ ] Add health check endpoint with dependency status (channels, agents, memory DB)
- [ ] Implement WebSocket reconnection logic with exponential backoff
- [ ] Add request/response validation middleware
- [ ] Add graceful shutdown with in-flight request draining

### Security Hardening (Current: 7.5/10 → Target: 9.5/10)
- [ ] Implement rate limiting per user/channel (token bucket or sliding window)
- [ ] Add duplicate message suppression (idempotency keys)
- [ ] Add persistent audit logging for all outbound actions and sensitive operations
- [ ] Enforce TLS in production mode (reject plaintext WebSocket unless explicitly configured)
- [ ] Add shell metacharacter sanitization for CommandExecTool inputs
- [ ] Add ONNX model integrity verification (SHA256 checksum validation on download)
- [ ] Add secret rotation support (reload tokens without restart)
- [ ] Security scanning in CI (govulncheck, gosec)

### DevOps & Release Engineering (Current: 6/10 → Target: 9/10)
- [ ] Add Dockerfile (multi-stage build, minimal runtime image)
- [ ] Add docker-compose.yml for local development (fractalbot + dependencies)
- [ ] Create Helm charts for Kubernetes deployment
- [ ] Add automated release workflow (semantic versioning, goreleaser, changelog generation)
- [ ] Add binary signing and checksum verification for release artifacts
- [ ] Add Dependabot/Renovate for automated dependency updates
- [ ] Add CI matrix testing (multiple Go versions, multiple OS)
- [ ] Add configuration hot-reload (watch config file, SIGHUP handler)

### Code Quality (Current: 8/10 → Target: 9.5/10)
- [ ] Add golangci-lint to CI with strict ruleset (errcheck, gocritic, exhaustive, etc.)
- [ ] Eliminate silent failures — return explicit errors instead of empty strings
- [ ] Remove stub/dead code (Canvas tool) or wire it up
- [ ] Complete Memory search tool implementation
- [ ] Add godoc for all exported types and functions (enforce via linter)
- [ ] Performance profiling and optimization (pprof baselines, allocation reduction)

---

## 🎮 Integration Points

### Required Integrations

1. **Telegram Bot API** - Initial channel support
2. **Slack API** - Enterprise collaboration
3. **Discord API** - Community engagement
4. **AI Model Providers** - OpenAI, Anthropic, others
5. **Browser Control** - Chrome CDP automation

### Optional Integrations

1. **WhatsApp** - Via BlueBubbles
2. **Signal** - Via signal-cli
3. **iMessage** - macOS only
4. **Matrix** - Decentralized communication
5. **Google Chat** - Enterprise G Suite

---

## 📊 Success Metrics

### Functional
- [ ] Telegram channel functional (end-to-end messaging)
- [ ] At least 3 channels fully operational (Telegram + Feishu + Slack or Discord)
- [ ] Plain user message returns concise final reply (no tmux/monitor raw output)
- [ ] Agent can proactively notify user without inbound trigger
- [ ] Broadcast can deliver to multiple configured channels safely
- [ ] Agent can execute basic tools
- [ ] Multi-agent coordination working
- [ ] Quality gates running

### Engineering Excellence
- [ ] Test coverage > 80% with coverage gate in CI (fail build if below threshold)
- [ ] Zero `govulncheck` / `gosec` findings in CI
- [ ] golangci-lint passes with strict config
- [ ] Structured logging throughout (no raw `log.Println`)
- [ ] Prometheus metrics + `/metrics` endpoint operational
- [ ] Dockerfile published, `docker run` works out of the box
- [ ] Automated releases via goreleaser with signed binaries
- [ ] Rate limiting active on all inbound channels
- [ ] Audit log covers all outbound actions
- [ ] First stable release (v1.0.0)

---

## 🚀 Release Plan

### v0.1.0-alpha (Current)
**Target**: Initial working gateway
**Scope**: WebSocket server + basic Telegram bot
**Status**: In development

### v0.2.0-beta
**Target**: Multi-channel + messaging UX baseline
**Scope**: Telegram + Slack + Discord transport completion + clean conversational replies
**Dependencies**: Phase 2 + Phase 2.5 (Conversational Reply Experience)

### v0.3.0-beta
**Target**: Outbound control plane
**Scope**: provider-agnostic outbound API + CLI send/notify/broadcast + policy guardrails
**Dependencies**: Phase 2.5 (Outbound Messaging API + Routing/Policy)

### v0.4.0-beta
**Target**: Engineering hardening
**Scope**: Structured logging + Prometheus metrics + rate limiting + audit logging + Dockerfile + CI security scanning + 80% test coverage
**Dependencies**: Technical Debt (Testing, Observability, Security, DevOps)

### v0.5.0-beta
**Target**: Agent runtime
**Scope**: Full agent lifecycle + tool execution
**Dependencies**: Phase 3 completion

### v1.0.0
**Target**: Stable multi-agent orchestration
**Scope**: Complete feature parity with Clawdbot MVP + full documentation + goreleaser automated releases + signed binaries
**Dependencies**: All phases complete + stability testing + engineering excellence metrics met

---

## 🤝 Contributing

Want to contribute? Check out:
- [Open Issues](https://github.com/fractalmind-ai/fractalbot/issues)
- [Pull Requests](https://github.com/fractalmind-ai/fractalbot/pulls)
- [Development Guide](CONTRIBUTING.md)

**Priority Areas**:
1. Messaging UX + routing control plane (Phase 2.5)
2. Slack/Discord channel completion (Phase 2)
3. Engineering hardening (Testing + Observability + Security + DevOps)
4. Agent runtime hardening (Phase 3)
5. Documentation: WebSocket protocol spec, architecture deep-dive, troubleshooting guide

---

*Last updated: 2026-02-20*
