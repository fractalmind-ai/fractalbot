# FractalBot Development Progress

## Status Update
**Date**: 2026-02-02
**Phase**: Phase 2 - Channel Integrations
**Overall Progress**: ~90%

---

## Recent Changes (merged PRs)

- **#43** Telegram polling backoff hardening
- **#47/#51/#53/#55/#57/#63/#65** Telegram + Feishu UX hardening (agent selection hints, allowlist parity, /whoami, /agents, error messaging, status tweaks)
- **#66** One-click installer (install.sh + README)
- **#72** Slack DM-only skeleton + /whoami onboarding
- **#73** Discord DM-only skeleton + /whoami onboarding
- **#74** Phase 3 runtime skeleton (default-deny tools)
- **#275** HTTP outbound message send API (`POST /api/v1/message/send`)
- **#276** CLI `fractalbot message send` via gateway API
- **#277** concise assign acknowledgement (`处理中…`) for plain conversation flow

---

## Progress Summary

| Module | Status | Completion |
|---------|--------|-------------|
| Phase 1 - Core Gateway | ✅ Complete | 100% |
| Phase 2 - Channel Integrations | 🚧 In Progress | 80% |
| Telegram Channel | ✅ Complete | 100% |
| Feishu/Lark Channel | ✅ Complete | 100% |
| Slack Channel | 🧪 Skeleton | 30% |
| Discord Channel | 🧪 Skeleton | 30% |
| Phase 3 - Agent Runtime | 🚧 Started | 10% |

---

## Current Capabilities

- Telegram + Feishu message routing with agent selection + allowlist UX parity
- Slack + Discord DM-only skeletons with /whoami onboarding
- Core commands: /help, /whoami, /agents, /ping, /status (+ admin lifecycle commands on Telegram)
- Safer error messaging and reply truncation
- One-click local install script (XDG-friendly)
- Phase 3 runtime skeleton with default-deny tool registry (echo/version)
- Outbound single-target send path (HTTP API + CLI send command)
- Concise default assign acknowledgement instead of raw monitor dump

---

## Issue #269 Snapshot (Partial Delivery)

Implemented:
- Natural default-agent chat now replies with concise acknowledgement (`处理中…`) after assign (#277)
- Outbound single-target message send API is available (`POST /api/v1/message/send`) (#275)
- CLI command for send is available (`fractalbot message send --channel --to --text`) (#276)

Still pending for full #269 scope:
- Channel-agnostic target model (`--to` currently numeric `int64`)
- Origin-channel/thread routing memory when target is omitted
- Proactive notify mode and multi-target broadcast/fan-out
- Broadcast safety controls (policy/rate-limit/audit trail)

---

## Next Steps

- **Messaging UX baseline (Phase 2.5)**: plain-message conversational replies, diagnostics separation, fallback/error polish
- **Outbound control plane (Phase 2.5)**: provider-agnostic send/notify/broadcast API + CLI
- **Routing/policy hardening (Phase 2.5)**: origin-channel continuity, allowlist guardrails, rate-limit/audit
- **Slack + Discord channels** (complete transport + non-DM support)
- **Agent Runtime (Phase 3)**: lifecycle, isolation, memory, tool execution

---

**Next Report**: TBD
