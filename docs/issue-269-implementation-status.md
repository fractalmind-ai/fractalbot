# Issue #269 Implementation Status

Issue: <https://github.com/fractalmind-ai/fractalbot/issues/269>

## Implemented (merged)

1. PR #275: HTTP outbound single-target send API
   - Endpoint: `POST /api/v1/message/send`
   - Payload: `channel`, `to`, `text`
2. PR #276: CLI send command
   - Command: `fractalbot message send --channel <name> --to <id> --text "<message>"`
   - Behavior: calls gateway API endpoint above
3. PR #277: concise default assign acknowledgement
   - Behavior: after successful assign, reply with `处理中…`
   - Purpose: avoid dumping raw monitor output into direct conversation replies

## Current Boundary

- Outbound target is still numeric (`to int64`) across CLI/API/channel interface.
- Slack/Discord outbound adapters are not yet using a channel-agnostic target string.
- Multi-target fan-out/broadcast is not implemented.
- Omitted-target reply routing (reuse last origin channel/thread) is not implemented.
- Routing memory, rate limit, duplicate suppression, and outbound audit trail are not implemented yet.

## Follow-up Work Needed To Close #269

1. Introduce channel-agnostic target model (string target + per-channel parsing/validation).
2. Add routing memory for default reply target when explicit target is absent.
3. Add proactive `notify` and `broadcast` modes with multi-target fan-out.
4. Add policy guardrails: allowlist, rate limit, duplicate suppression, audit logs.
