# Agent Heartbeats

FractalBot can wake supported Agent Runtimes on explicit-timezone cron schedules. A heartbeat asks an Agent to inspect and advance its work; it is not a runtime readiness probe and it is not a synthetic channel message.

Supported targets are `ohMyCode`, `codexAppCDP`, and `claudeDesktop`. Each job selects its Runtime and Agent directly, independently of the inbound `agents.router` setting.

## Configuration

The target Runtime must be enabled, and the job's Agent must be accepted by its `defaultAgent` or `allowedAgents` configuration.

```yaml
agents:
  heartbeat:
    enabled: true
    statePath: "./workspace/heartbeat-state.json"
    maxConcurrent: 2
    jobs:
      - id: "cloudbank-main"
        runtime: "codexAppCDP"
        agent: "main"
        text: "Inspect the current project and continue any actionable work."
        cron: "*/10 * * * *"
        timezone: "Asia/Shanghai"
        agentCronProfiles:
          idle: "0 * * * *"
          deep-idle: "0 */6 * * *"
        resetCronOnInbound: true
```

`text` is the complete inline instruction. Heartbeat prompt files are not supported. Cron expressions use the standard five-field format, and every job requires an IANA timezone.

The configured `cron` remains the default. `agentCronProfiles` contains the only alternative schedules an Agent may select; arbitrary Agent-provided cron expressions are rejected.

## Schedule adjustment

A normal heartbeat requires no callback. Only when the Agent determines that there is no actionable work should it reduce the frequency:

```bash
fractalbot heartbeat cron set \
  --job cloudbank-main \
  --profile idle \
  --reason "no actionable tasks"
```

Setting the active profile again is a successful no-op. Restore the configured default with:

```bash
fractalbot heartbeat cron reset --job cloudbank-main
```

The equivalent loopback-only API is:

```http
PUT /api/v1/heartbeat/jobs/cloudbank-main/cron
Content-Type: application/json

{"profile":"idle","reason":"no actionable tasks"}
```

```http
DELETE /api/v1/heartbeat/jobs/cloudbank-main/cron
```

When `resetCronOnInbound` is true, a normal user message successfully routed to the same Runtime and Agent also restores the default cron. Rejected, malformed, or unrelated messages do not reset it.

## Delivery behavior

- One run per job may be in flight; overlapping ticks are skipped.
- `maxConcurrent` limits dispatches across all jobs.
- The effective profile and delivery telemetry are written atomically to `statePath`.
- Restart preserves the effective profile but calculates the next future occurrence. Missed heartbeats are never replayed.
- Codex App and Claude Desktop inbox fallback uses a stable key per job. A newer queued heartbeat replaces the older unconsumed heartbeat.
- Runtime delivery errors receive a bounded exponential-backoff retry. The final failure is recorded without changing the Agent-selected schedule.

## Status

`GET /status` includes a redacted `heartbeat` section with configured and effective cron, timezone, next run, in-flight state, last dispatch result, and schedule-change audit fields.

```bash
curl -sS http://127.0.0.1:18789/status | python3 -m json.tool
```

The inline instruction is not included in status output.
