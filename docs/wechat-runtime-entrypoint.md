# 微信当前运行时接入入口（fractalmind-ai）

> Status: draft
> Updated: 2026-03-22 23:00 PDT
> Purpose: 在不重新安装 OpenClaw core 的前提下，确认 `fractalmind-ai` 当前运行时中微信能力应挂接的真实宿主与入口。

## 1. 结论

基于当前工作区代码与本机正在运行的 FractalBot 实例，可确认：

- **当前运行时宿主是 FractalBot gateway 进程**，不是额外的 OpenClaw core 进程
- 若后续要在当前系统内接入微信，**现成的挂接入口位于 FractalBot 的 channel manager / gateway 层**
- 也就是说，O7-KR2 下一步要验证的“真实入口”，当前应优先围绕：
  - `cmd/fractalbot/main.go`
  - `internal/gateway/server.go`
  - `internal/channels/manager.go`
  - `internal/channels/wechat.go`

这是一份**运行时入口确认记录**，不是微信最小闭环已经跑通的证明。

## 2. 代码证据

### 2.1 启动入口是 FractalBot gateway

`cmd/fractalbot/main.go` 中：

- 读取 `config.yaml`
- 调用 `gateway.NewServer(cfg)`
- 再执行 `server.Start(ctx)`

说明当前主进程的宿主模型是：

**FractalBot CLI / gateway server → channel manager → agent manager**

### 2.2 channel 挂接点在 gateway.NewServer

`internal/gateway/server.go` 中：

- `channelManager := channels.NewManager(cfg.Channels, cfg.Agents)`
- `agentManager := agent.NewManager(cfg.Agents)`
- `agentManager.ChannelManager = channelManager`
- `channelManager.SetHandler(agentManager)`
- `Start()` 时执行 `s.agentManager.ChannelManager.Start(ctx)`

说明：

- 所有 channel 都是在 **FractalBot gateway 启动流程** 中被创建和启动
- inbound message 的统一进入点是 channel manager → agent manager
- 因此，若微信要在当前运行时中接入，最自然的宿主就是这个 channel 抽象层

### 2.3 wechat 已有待接入 scaffold

`internal/channels/manager.go` 与 `internal/channels/wechat.go` 表明：

- `wechat` 已被纳入 channel manager 的注册/启动范围
- 当前已存在 callback listener scaffold、query precheck、XML parsing、protocol mapping、handler routing 等 KR2 草稿实现

因此从代码结构上说：

**当前运行时不是“没有微信入口”，而是“已有 FractalBot 内部入口，但最小闭环还未完成验证”。**

## 3. 运行时证据（本机当前实例）

### 3.1 当前 gateway 绑定

从本机 `~/.config/fractalbot/config.yaml` 读取到的非敏感配置显示：

- `gateway.bind = 127.0.0.1`
- `gateway.port = 18789`

### 3.2 当前已启用 channel

同一份本地配置的非敏感字段显示：

- `telegram.enabled = true`
- `imessage.enabled = true`
- `slack / feishu / discord = false`
- 当前配置中**尚未启用 wechat channel**

### 3.3 当前运行中的 gateway 状态

本机 `curl http://127.0.0.1:18789/status` 返回：

- `telegram` 运行中
- `imessage` 运行中
- agent workspace 已配置
- 当前没有 `wechat` 运行实例出现在 `/status`

这进一步说明：

1. 当前真实宿主确实是本地 FractalBot gateway
2. 当前环境下微信还未被配置/启用
3. O7-KR2 的下一步不是“寻找另一个宿主”，而是在这个运行时里补齐微信最小闭环验证

## 4. 对 O7-KR2 的直接意义

这次确认把问题收敛为更具体的下一步：

1. **宿主已确认**：FractalBot gateway / channel manager
2. **当前缺口已确认**：wechat 尚未在当前配置中启用，也未完成 end-to-end 证据
3. **下一步验证动作应聚焦**：
   - 明确采用 callback 还是 polling 作为当前运行时内的第一条验证路径
   - 若继续走 FractalBot scaffold，则补“真实启动 + 状态可见 + 单次收消息/回消息证据”
   - 若转为参考 `picoclaw` 的 polling 模型，则需先定义该模型如何接入现有 channel manager

## 5. 当前边界

本记录只证明：

- 当前 `fractalmind-ai` 运行时里，微信能力应优先挂接到 **FractalBot gateway/channel** 层

本记录**不证明**：

- callback 模式一定是最终路线
- polling 模式无法接入当前架构
- 微信最小闭环已经完成

后续仍需继续补：

- 真实启动/配置证据
- inbound/outbound 最小闭环证据
- route 收敛说明（FractalBot 适配层 vs 其他桥接方式）
