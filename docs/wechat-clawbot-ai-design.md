# WeChat Official Support for ClawBot AI — KR1 初版设计

> Status: draft
> Updated: 2026-03-23 (UTC+8)
> Scope: 为 `fractalmind-ai/fractalbot` 设计“微信官方能力”接入方案，支撑 ClawBot AI 最小可用闭环。

## 1. 目标

让 FractalBot 新增 1 条基于**微信官方能力**的通道，满足：

1. 能接收微信侧消息并标准化成 FractalBot `protocol.Message`
2. 能将 Agent 回复回写到微信侧
3. 能把微信用户身份、会话上下文、路由目标注入给 oh-my-code / agent-manager
4. 能处理官方鉴权、回调验证、异步回复、失败重试

## 2. 已核验的官方资料（用于 KR1 边界判断）

### 微信服务号 / 公众号
- `Receiving_standard_messages`：微信服务号文档《文本消息》
- `Passive_user_reply_message`：微信服务号文档《回复文本消息》

这两份文档至少确认了以下接口形态：
- 存在**HTTP 回调接收消息**模式
- 入站消息包含 `ToUserName / FromUserName / MsgType / MsgId / CreateTime` 等字段
- 平台支持**被动回复**文本消息

### 企业微信
- 企业微信开发者中心《概述》
- 企业微信开发者中心《基本概念介绍》
- 企业微信开发者中心《消息推送配置说明》

这三份文档至少确认了以下接入形态：
- 企业微信存在**服务器回调 / 消息推送配置**
- 官方能力是“企业应用 / 组织内账号”导向，而不是个人微信号自动化

## 3. 初步边界判断

### 3.1 支持范围
优先支持两类官方能力：

1. **企业微信应用（WeCom）**
   - 适合组织内部使用、员工/运营/团队场景
   - 更符合 FractalBot 当前“agent routing / 运维控制 / 内部协作”定位

2. **微信服务号 / 公众号（MP Service Account）**
   - 适合对外用户咨询、客服、AI 助手入口
   - 更适合 “ClawBot AI 面向外部用户” 的场景

### 3.2 不支持范围
- **个人微信号自动化**不纳入官方支持方案
- 若用户需求是“个人号机器人”，应单独定义为非官方实验能力，不进入本 OKR 成功标准

> 说明：上面这条是当前设计判断，后续仍需基于官方文档逐条核验。

## 4. 推荐路线

### 推荐顺序

#### 路线 A：先做企业微信应用
原因：
- 与 FractalBot 现有“工作流 / agent-manager / allowlist / 组织内协作”更一致
- 官方回调 + 服务端 API 形态更接近现有 Telegram webhook / Feishu / Slack 通道模型
- 对“ClawBot AI 先内部用起来”更现实

#### 路线 B：再扩展服务号 / 公众号
原因：
- 对外触达更强
- 但通常需要补充菜单、客服、消息模板、关注关系等平台约束设计

## 5. FractalBot 抽象设计

## 5.1 Channel 命名策略
推荐不要直接把 channel 名死写成 `wecom` 或 `mp`，而是抽象为：

- **外部统一名**：`wechat`
- **内部 provider**：`wecom` / `mp_service`

好处：
- 对 Agent / 网关 / 路由层保持一个稳定 channel 名
- 允许后续在同一 channel 下扩展多个微信官方 provider

## 5.2 配置草案

```yaml
channels:
  wechat:
    enabled: false
    provider: "wecom" # wecom | mp_service

    # inbound callback
    callbackListenAddr: "127.0.0.1:18810"
    callbackPath: "/wechat/callback"
    callbackToken: "replace-me"
    callbackEncodingAESKey: "replace-me-if-required"

    # outbound auth
    corpId: ""
    corpSecret: ""
    agentId: ""
    appId: ""
    appSecret: ""

    # routing
    defaultAgent: "main"
    allowedAgents:
      - "main"
      - "qa-1"

    # reply policy
    syncReplyTimeoutSeconds: 4
    asyncSendEnabled: true
    accessTokenCacheFile: "./workspace/wechat.token.json"
```

说明：
- `provider` 决定具体官方实现
- `callback*` 统一微信官方回调入口配置
- `corpId/corpSecret/agentId` 面向企业微信
- `appId/appSecret` 面向服务号/公众号
- `syncReplyTimeoutSeconds` 用于区分快速被动回复与异步回写

## 5.3 标准化消息结构

对齐现有 Telegram / Feishu / iMessage 的 `protocol.Message` 数据约定，建议入站转换为：

```json
{
  "channel": "wechat",
  "provider": "wecom",
  "text": "用户消息",
  "agent": "main",
  "chat_id": "conversation-or-external-user-id",
  "user_id": "wechat-user-id",
  "username": "optional-display-name",
  "message_id": "official-message-id",
  "chatType": "dm",
  "event": "message|subscribe|menu_click"
}
```

补充字段建议：
- `open_id` / `union_id` / `external_userid` / `from_user_name`
- `raw_payload`
- `reply_mode`：`passive` / `async_api`
- `provider_agent_id`

## 6. 路由与身份映射

### 6.1 路由策略
沿用当前 FractalBot inbound routing contract：
- 微信入口消息 → 解析出文本 / 事件 / 身份
- 根据命令前缀或默认配置解析 `selected_agent`
- 将 `channel/chat_id/user_id/username/selected_agent` 注入 assign prompt

### 6.2 身份映射原则
- `chat_id`：优先使用“可稳定标识会话”的官方会话 ID / 用户 ID
- `user_id`：使用平台原生用户标识
- `username`：仅作展示，不参与权限判断
- 权限控制：继续沿用 allowlist / default-deny 思路

## 7. 收发模型

### 7.1 入站
- 不做 polling
- 统一采用**官方回调推送**
- FractalBot 内部起一个 HTTP handler，模式参考现有 Telegram webhook server

### 7.2 出站
- 若 Agent 在 `syncReplyTimeoutSeconds` 内返回，优先走同步回复
- 若超时：
  1. 先返回“处理中”或平台允许的兜底应答
  2. Agent 完成后走官方发送 API 异步回写

### 7.3 失败处理
- access token 获取失败：记录 telemetry + 指数退避
- 回调签名失败：拒绝处理并记安全日志
- 下游 Agent 超时：返回统一 fallback 文案
- 微信 API 失败：重试有限次数，必要时写入 dead-letter / 本地待补发队列

## 8. 与现有代码的贴合点

当前 FractalBot 已有可复用模式：
- `internal/channels/manager.go`：通道注册与启动
- `internal/channels/telegram.go`：webhook / polling 双模式与路由注入
- `internal/channels/feishu.go`：官方平台 SDK / 事件转协议消息
- `internal/channels/imessage.go`：将外部消息标准化为统一 `protocol.Message`
- `pkg/protocol/message.go`：统一消息协议

因此新增 `internal/channels/wechat.go` 是最自然路径。

## 9. KR2 最小实现切片建议

按最短闭环拆成 4 步：

1. **配置 + status 暴露**
   - 配置解析
   - `/status` 显示 wechat channel 状态

2. **企业微信 callback 收消息**
   - 启动 callback server
   - 验证签名 / 基础参数
   - 解析文本消息并转 `protocol.Message`

3. **异步发消息**
   - access token 获取与缓存
   - 调用官方发送 API

4. **Agent 路由闭环**
   - 注入 routing context
   - 完成“微信消息 → agent → 微信回写”

## 10. 待确认问题

1. 本项目的“ClawBot AI”目标用户是**内部团队**还是**外部用户**？
   - 内部优先：先做企业微信
   - 外部优先：先做服务号

2. 是否接受“首版只支持文本消息 + 单轮会话”？
   - 建议接受，便于尽快闭环

3. 是否需要把“菜单点击 / 订阅事件 / 图片消息”纳入首版？
   - 建议首版不做，只做文本消息

## 11. 当前结论

- **KR1 推荐结论**：先按 `wechat(provider=wecom)` 设计落地，优先完成企业微信官方通道闭环
- **实现策略**：复用 Telegram webhook server + Feishu 官方 SDK / 事件处理模式 + 现有 routing contract
- **首版范围**：文本消息、组织内 DM、agent 路由、异步回写、最小权限控制
- **后续扩展**：在同一 `wechat` channel 下增加 `mp_service` provider
