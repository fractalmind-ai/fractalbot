# sipeed/picoclaw 微信渠道参考笔记

> Status: draft
> Updated: 2026-03-23 (UTC+8)
> Purpose: 记录 `https://github.com/sipeed/picoclaw` 已接入微信渠道的证据，作为 O7 后续在 `fractalmind-ai` 当前运行时内适配官方微信能力的外部参考。

## 1. 结论

`picoclaw` **已经接入微信渠道**，而且不是文档占位，而是包含完整的可运行实现。

## 2. 关键证据

### 2.1 渠道源码目录完整存在

仓库下存在完整 `pkg/channels/weixin/` 目录：

- `pkg/channels/weixin/weixin.go`
- `pkg/channels/weixin/api.go`
- `pkg/channels/weixin/auth.go`
- `pkg/channels/weixin/media.go`
- `pkg/channels/weixin/state.go`
- `pkg/channels/weixin/types.go`
- `pkg/channels/weixin/weixin_test.go`

这说明其微信能力已经落到正式 channel 实现、状态管理、媒体处理与测试层，而不是 README 草稿。

### 2.2 渠道已在框架中注册

`pkg/channels/weixin/weixin.go` 中注册了 `weixin` factory，并在 `Start()` 中启动轮询逻辑。

这表明：

- 微信渠道是框架正式支持的一部分
- 不是孤立的示例代码
- 运行时预期会真实拉取消息并进入统一 channel 抽象

### 2.3 接入方式是腾讯 iLink API + 扫码登录

从 `pkg/channels/weixin/api.go` 与 `pkg/channels/weixin/auth.go` 可确认：

- 默认 `BaseURL = https://ilinkai.weixin.qq.com/`
- 提供 `GetUpdates`、`SendMessage`、`GetUploadUrl`、`GetConfig`、`SendTyping`、`GetQRCode`、`GetQRCodeStatus` 等 API 封装
- 登录流程是终端展示二维码 + 轮询扫码状态，最终拿到 `botToken / userID / accountID / baseUrl`

这意味着它采用的是：

**腾讯 iLink API + 原生扫码登录**

而不是公众号/企业微信 callback 回调模型。

### 2.4 收消息机制是 long polling，不是 webhook

从 `pkg/channels/weixin/weixin.go` 可确认：

- `Start()` 会启动 `pollLoop()`
- `pollLoop()` 持续调用 `GetUpdates(...)`
- 使用 `get_updates_buf` 维护游标
- `pkg/channels/weixin/state.go` 持久化轮询状态

所以它的消息接入模型是：

**REST API + long polling + cursor state**

而不是传统 webhook / callback 推送。

### 2.5 配置与文档都是正式形态

`pkg/config/config.go` 与 `pkg/config/defaults.go` 中存在 `WeixinConfig`：

- `Enabled`
- `Token`
- `BaseURL`
- `CDNBaseURL`
- `Proxy`
- `AllowFrom`
- `ReasoningChannelID`

默认值里直接包含：

- `BaseURL: https://ilinkai.weixin.qq.com/`
- `CDNBaseURL: https://novac2c.cdn.weixin.qq.com/c2c`

文档层也明确列出微信渠道：

- `README.md`
- `docs/channels/weixin/README.md`
- `docs/channels/weixin/README.zh.md`
- `docs/chat-apps.md`
- `docs/zh/chat-apps.md`

## 3. 对 O7 的启发

`picoclaw` 的实现给出一个很重要的信号：

1. **微信渠道不一定只能走 webhook / callback**
   - 还可以走扫码登录后的 API polling 路线

2. **“当前运行时内适配官方微信能力”可以参考成熟 channel 分层**
   - 认证层：扫码 / token / baseUrl
   - API 层：updates / send / media / typing
   - 状态层：cursor / persistence
   - 渠道路由层：统一映射到现有 message/channel 抽象

3. **此前 FractalBot 自研 callback 草稿并不覆盖这一路线**
   - 说明 O7-KR2 需要补做“polling 型微信接入”的设计判断
   - 不能只围绕 callback 能力推进

## 4. 当前可直接参考的代码点

建议后续重点参考：

- `pkg/channels/weixin/weixin.go`：渠道生命周期、poll loop、消息分发
- `pkg/channels/weixin/api.go`：iLink REST API 封装
- `pkg/channels/weixin/auth.go`：二维码登录与 token 获取
- `pkg/channels/weixin/state.go`：游标与状态持久化
- `pkg/config/config.go`：配置结构设计

## 5. 当前边界

该参考结论的意义是：

- 证明已有开源项目在真实实现“微信渠道”时采用了 **iLink API + QR login + long polling**
- 为 O7 提供架构参考

但它**不自动等于**我们当前运行时可以直接复用同一路线；后续仍需单独确认：

1. `fractalmind-ai` 当前运行时是否更适合 callback 模式、polling 模式，或两者混合
2. 是否能直接对接同类 API / 鉴权能力
3. 是否需要把 FractalBot 定义为桥接层而非微信协议层
