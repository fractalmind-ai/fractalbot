# O7 KR2 polling-first 最小方案

> Status: draft
> Updated: 2026-03-23 08:55 PDT
> Purpose: 以 `picoclaw` 的 weixin channel 为主要参考，收敛当前运行时里的 polling-first 第一阶段方案。

## 1. 参考来源

优先参考：

- `projects/fractalmind-ai/fractalbot/docs/wechat-picoclaw-reference.md`
- `sipeed/picoclaw/pkg/channels/weixin/weixin.go`
- `sipeed/picoclaw/pkg/channels/weixin/api.go`
- `sipeed/picoclaw/pkg/channels/weixin/auth.go`
- `sipeed/picoclaw/pkg/channels/weixin/state.go`

## 2. 第一阶段最小对象模型

建议先定义以下最小结构：

### 2.1 Auth / session
- `base_url`
- `bot_token`
- `user_id`
- `account_id`
- `session_updated_at`

### 2.2 Polling state
- `get_updates_buf` / 等价 cursor
- `last_poll_at`
- `last_message_id`
- `last_error_at`

### 2.3 Runtime config
- `enabled`
- `mode: polling`
- `baseURL`
- `token`
- `stateFile`
- `defaultAgent`
- `allowedAgents`

## 3. 第一阶段最小运行流程

1. 启动 FractalBot gateway
2. 初始化 wechat polling channel
3. 读取本地 token/baseUrl/state
4. 进入 poll loop
5. 把 updates 映射为 `protocol.Message`
6. 交给现有 handler / agent manager

## 4. 第一阶段不追求的东西

- callback listener 证明
- 真实企业微信 / 服务号回调
- 多账号并发
- 完整媒体能力
- 生产级重试策略

## 5. 第一阶段需要的证据

- polling 配置片段
- 轮询启动日志
- 一次成功的 updates 拉取或等价模拟证据
- 一次消息映射到 handler 的证据
- state file / cursor 更新证据
