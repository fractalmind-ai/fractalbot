# O7 KR2 polling-first gap audit

> Status: draft
> Updated: 2026-04-04 05:01 PDT
> Purpose: 把当前 polling-first 阻塞从“缺私有配置”收敛为更精确的实现缺口，避免下一轮 heartbeat 继续误判为只差 token/baseURL。

## 结论

当前 `fractalmind-ai/fractalbot` 仓库里，wechat 代码路径仍是 **callback-first scaffold**，还没有真正进入 polling-first 最小闭环。

因此 O7 KR2 当前第一阻塞 **不只是** 私有 `config.yaml` / `baseURL` 映射缺失，而是：

1. `WeChatConfig` 里还没有 polling-first 所需字段
2. `ChannelManager` 还没有把 wechat 按 polling 模式初始化
3. `WeChatBot` 还没有 poll loop / state persistence / update→handler 路径

## 代码证据

### 1. 配置结构仍是 callback-only

`internal/config/config.go` 的 `WeChatConfig` 当前只有：
- `provider`
- `callbackListenAddr`
- `callbackPath`
- `callbackToken`
- `callbackEncodingAESKey`
- `corpId/corpSecret/agentId`
- `appId/appSecret`
- `defaultAgent/allowedAgents`
- `syncReplyTimeoutSeconds/asyncSendEnabled/accessTokenCacheFile`

未见 polling-first 文档里提到的字段：
- `mode`
- `baseURL`
- `token`
- `stateFile`
- `pollIntervalSeconds`

### 2. manager 初始化仍只配置 callback

`internal/channels/manager.go` 当前对 wechat 只做：
- `NewWeChatBot(provider)`
- `ConfigureCallback(...)`
- `Register(bot)`

未见 polling 模式分支，也未见任何 `baseURL/stateFile/pollInterval` 注入。

### 3. wechat bot 仍是 callback listener scaffold

`internal/channels/wechat.go` 当前能力集中在：
- callback server lifecycle
- callback request parsing
- handshake / signature verification
- XML → `protocol.Message` 映射草稿

未见：
- polling client
- updates 拉取循环
- cursor/state 文件写入
- polling update → handler 的最小闭环

## 对现有文档的影响

`docs/wechat-polling-config-overlay.example.yaml` 与 `docs/wechat-kr2-polling-proof-commands.sh` 现在更像是 **目标配置草案**，而不是可直接运行的现状说明。

下一轮执行 KR2 时，不能再把“补私有 token/baseURL”当成唯一前置条件；否则会继续卡在代码路径不支持 polling。

## 建议的最小下一刀

按最小闭环拆成 3 步：

### Slice 1 — 配置层
补齐 `WeChatConfig` polling-first 字段：
- `mode`
- `baseURL`
- `token`
- `stateFile`
- `pollIntervalSeconds`

### Slice 2 — 初始化层
让 `ChannelManager` 能根据 `channels.wechat.mode` 决定：
- callback-first
- polling-first

### Slice 3 — 运行证据层
在 `WeChatBot` 中补最小 polling loop：
- 读取 updates / 或 mock updates
- 产出一条进入 handler 的证据
- 写出 `stateFile`
- 留下 before/after cursor 证据

## 当前 heartbeat 应如何表述

更准确的 blocker 应为：
- wechat config schema 仍缺 polling-first 字段
- wechat runtime 仍未实现 poll loop / state persistence
- 私有 token/baseURL 仍需要，但不是唯一缺口
