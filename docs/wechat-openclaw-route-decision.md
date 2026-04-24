# 微信 ClawBot / OpenClaw 官方路线决策

> Status: draft
> Updated: 2026-03-23 (UTC+8)
> Purpose: 纠正 O7 技术路线，避免继续沿着错误的 FractalBot 自研微信 callback 方向投入。
> Supersedes: `docs/wechat-clawbot-ai-design.md` 中“FractalBot 直接实现微信官方通道”为默认主路线的假设。

## 1. 结论先行

**修正后主路线：当前环境以 `https://github.com/fractalmind-ai` 驱动运行为准；腾讯官方 `@tencent-weixin/openclaw-weixin` / `@tencent-weixin/openclaw-weixin-cli` 只作为能力边界与实现参考，不再默认要求安装 OpenClaw core。**

在完成官方插件最小闭环验证前，**不再把 FractalBot 自研微信 callback / channel 作为默认推进方向**。

FractalBot 后续只保留两种可能角色，且都必须在官方方案验证后再决定：

1. **适配层**：把官方 OpenClaw 微信插件与现有 FractalBot / agent-manager 工作流对接起来
2. **归档对象**：若官方方案已充分满足需求，则停止继续扩展当前自研 wechat 通道草稿

## 2. 已核实事实

### 2.1 官方包与发布时间
已核实腾讯官方 npm scope 在 **2026-03-21** 发布：

- `@tencent-weixin/openclaw-weixin@1.0.2`
- `@tencent-weixin/openclaw-weixin-cli@1.0.2`

### 2.2 官方元数据
已核实包元数据：

- `author = Tencent`
- maintainers 使用多个 `@tencent.com` 邮箱

### 2.3 官方参考接法（仅作边界参考）
README 给出的官方路径为：

```bash
npx -y @tencent-weixin/openclaw-weixin-cli install
```

或手动：

```bash
openclaw plugins install "@tencent-weixin/openclaw-weixin"
openclaw config set plugins.entries.openclaw-weixin.enabled true
openclaw channels login --channel openclaw-weixin
openclaw gateway restart
```

### 2.4 官方能力边界（当前已知）
从官方 README 可确认：

- 支持扫码登录
- 支持多个微信账号同时在线
- 支持通过 HTTP JSON API 与后端通信
- 默认接入点是 **OpenClaw 插件体系**，而不是 FractalBot channel 抽象


### 2.5 用户最新运行时约束（2026-03-23）
用户已明确说明：

- 当前助手是 **`https://github.com/fractalmind-ai` 驱动运行**
- **不要再安装 OpenClaw**
- 之前已经卸载过 OpenClaw

因此，后续所有实施与验证都必须遵守以下约束：

1. 不再把“安装/升级 OpenClaw core”作为默认操作
2. 不再把 upstream OpenClaw runtime 视为当前环境的真实宿主
3. 官方 `openclaw-weixin` 包主要用于确认官方能力边界、接口形态与兼容性要求
4. 真正的落地验证必须回到 `fractalmind-ai` 当前运行时与仓库边界内完成

### 2.6 开源参考补充：`sipeed/picoclaw` 已有微信渠道实现

已核实 `https://github.com/sipeed/picoclaw` 在 **2026-03-23（UTC+8）** 时，仓库中存在完整 `pkg/channels/weixin/` 实现，而不是仅有文档占位。

当前已确认的关键特征：

- 渠道已在框架中注册并启用
- 认证方式是 **腾讯 iLink API + 二维码登录**
- 收消息机制是 **long polling (`GetUpdates`)**，而不是 webhook / callback
- 存在状态持久化、媒体上传、typing 等配套实现

这条证据的重要性在于：**现有开源“微信渠道”实现并不一定走 callback 路线，也可能走扫码登录后的 API polling 路线。**

因此，O7 后续在 `fractalmind-ai` 当前运行时中确认“真实微信接入入口”时，必须同时评估：

1. callback 型接入
2. polling 型接入
3. 二者混合的桥接层设计

详细参考见：`projects/fractalmind-ai/fractalbot/docs/wechat-picoclaw-reference.md`

## 3. 为什么原路线有偏差

原来的默认假设是：

- 在 `fractalmind-ai/fractalbot` 内新增 `channels.wechat`
- 直接复刻 Telegram / Feishu 模式，实现 callback、签名校验、消息解析、发送 API、回写闭环

这个方向的问题在于：

1. **忽略了官方已经给出的现成插件入口**
   - 如果官方已有 OpenClaw 微信插件，自研通道不应再被默认视为 MVP

2. **抽象层级可能不对**
   - 官方插件的宿主是 OpenClaw，而不是 FractalBot
   - FractalBot 继续做微信 channel，可能是在错误层级重复造轮子

3. **验证顺序倒置**
   - 正确顺序应是：先验证官方方案能不能跑通，再决定是否需要适配层
   - 而不是先实现一套自研 callback，再回头看官方方案

## 4. 两条可选路线对比

### 路线 A：直接依赖 upstream OpenClaw 插件直连（不再默认推荐）

**描述**
- 直接采用腾讯官方 `openclaw-weixin` 插件
- 通过 upstream OpenClaw CLI / gateway 完成安装、登录、收发消息

**优点**
- 与官方发布路径一致，方向风险最低
- 接入成本更低，能最快验证真实可用性
- 避免在签名校验、扫码登录、会话维护、协议细节上重复造轮子

**缺点**
- 当前工作流若强依赖 FractalBot，需要补一层系统集成
- 我们对官方插件内部可扩展性还缺少一手验证

**何时采用**
- 仅在确认当前环境确实回到 upstream OpenClaw runtime 时采用
- 在当前用户约束下，不作为默认起点

### 路线 B：FractalBot 适配层

**描述**
- 不让 FractalBot 直接实现微信官方通道
- 而是在官方 OpenClaw 微信插件之上，补一个 FractalBot/agent-manager 的桥接层

**优点**
- 复用已有 FractalBot 路由、agent-manager、状态观测能力
- 保持现有内部工作流连续性

**缺点**
- 必须以官方方案已经验证可用为前提
- 适配层边界如果设计不好，仍可能重复实现官方能力

**何时采用**
- 只有在官方插件验证通过后，且确认确有 FractalBot 集成需求时才采用


### 路线 C：fractalmind-ai 驱动运行时内适配官方微信能力（当前默认推荐）

**描述**
- 以当前 `fractalmind-ai` 驱动运行时为宿主
- 参考腾讯官方微信插件/CLI 暴露出来的能力边界、协议与登录流程
- 在现有仓库和运行边界内完成最小闭环验证

**优点**
- 符合用户最新运行时约束
- 不再误把安装 OpenClaw core 当成前提
- 能更准确回答“当前系统到底如何接微信”

**缺点**
- 需要重新界定官方包在当前架构中的角色：参考实现、协议参考还是直接可复用组件
- 目前还缺少一份面向 fractalmind-ai 运行时的明确接入草图

**何时采用**
- 当前立即采用
- 作为 O7 后续推进的默认起点

## 5. 当前决策

### 5.1 立即生效的默认决策
- **默认主路线 = fractalmind-ai 驱动运行时内适配官方微信能力（路线 C）**
- **不再默认安装 OpenClaw core**
- **暂停继续扩展 FractalBot 自研微信 callback 的范围**

### 5.2 当前自研草稿的处理原则
对 `fractalmind-ai/fractalbot` 中现有未提交 wechat 草稿代码：

- 可保留为“已探索实现草稿”
- 但 **不再作为 O7 当前成功标准的主路径**
- 是否继续保留、重构为适配层，推迟到 KR3 决定

## 6. O7 的正确推进顺序

1. **KR1**：完成路线决策文档
   - 说明为什么默认采用官方插件
   - 说明 FractalBot 是否还需要介入

2. **KR2**：完成基于当前运行时的最小闭环验证
   - 明确实际宿主与接入入口
   - 登录或等价鉴权
   - 收消息
   - 回消息
   - 留下命令/日志/截图证据

3. **KR3**：收敛系统边界
   - 如果官方方案足够：归档或冻结自研通道草稿
   - 如果确有内部集成需求：把 FractalBot 定义为适配层，而不是微信协议实现层

## 7. 需要重点验证的问题

### 7.1 功能问题
- 官方插件是否能稳定完成消息接收与回复？
- 多账号同时在线是否真的可用？
- 上下文隔离是否满足我们的 agent routing 需求？

### 7.2 集成问题
- OpenClaw 与现有 FractalBot / agent-manager 的关系如何定义？
- 是否需要统一 outbound / routing / observability？
- 是否存在从 OpenClaw 向现有工作流桥接的最小接口？

### 7.3 风险问题
- 官方插件更新节奏如何？
- 登录态与协议稳定性如何？
- 若官方插件不可用，是否需要回退到适配层或其他方案？

## 8. 对 heartbeat 的约束

后续 heartbeat 在 O7 上只能优先做以下事情：

1. 补官方路线文档
2. 做官方插件验证
3. 做路线收敛

**不得再默认把“继续写 FractalBot 自研微信 callback”当作 O7 的主推进动作。**
