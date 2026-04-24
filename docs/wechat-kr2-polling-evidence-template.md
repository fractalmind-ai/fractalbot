# O7 KR2 polling-first 运行证据模板

> Status: draft
> Updated: 2026-03-23 11:00 PDT
> Purpose: 为 polling-first 第一轮验证准备统一证据模板，确保配置、轮询启动、state/cursor 与消息映射都能一次性归档。

## 1. 环境信息
- Date:
- Host:
- Working tree / branch:
- FractalBot config path:
- WeChat mode: polling
- baseURL source:
- token source:
- state file path:

## 2. 启动命令
```bash
cd projects/fractalmind-ai/fractalbot
fractalbot --config <config-path>
```

## 3. 轮询配置证据
- Config snippet:
- Poll interval:
- State file path:
- defaultAgent / allowedAgents:

## 4. `/status` 证据
- Raw `/status` JSON:
- `channels.wechat.mode`:
- `channels.wechat.polling.base_url_configured`:
- `channels.wechat.polling.token_configured`:
- `channels.wechat.polling.state_file_configured`:
- `channels.wechat.polling.cursor_present`:
- `channels.wechat.polling.cursor_preview`:
- `channels.wechat.polling.state_file_exists`:
- `channels.wechat.polling.last_poll_at`:
- `channels.wechat.polling.last_poll_messages`:

## 5. 启动日志证据
- Did polling channel initialize?
- Did it enter poll loop?
- Key log lines:

## 6. updates / mock updates 证据
- Request or mock input:
- Raw output:
- Whether message was mapped into `protocol.Message`:
- Whether handler received it:

## 7. state / cursor 证据
- State file exists?
- Cursor field name:
- Before:
- After:

## 8. 结论
- Did we prove polling channel startup?
- Did `/status` expose runtime polling telemetry?
- Did we prove one update can enter the handler path?
- Did we prove state persistence?
- Remaining gap to full official capability:
