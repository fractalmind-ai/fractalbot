# O7 KR2 第一条验证路径决策

> Status: draft
> Updated: 2026-03-23 08:55 PDT
> Purpose: 基于用户最新要求，把 KR2 第一条验证路径从 callback-first 切换为 polling-first。
> Supersedes: 本文件此前关于 “callback-first” 的结论。

## 1. 结论

**KR2 的第一条验证路径，改为优先选择 `polling-first`，`callback` 放到第二期。**

也就是说，下一步最小闭环不再优先围绕本地 callback listener，而是优先围绕：

1. iLink / 类 iLink API 的接入边界
2. 登录态 / token / baseUrl / account state
3. polling loop（`GetUpdates` / 等价接口）
4. cursor / state persistence
5. 将 polling 消息映射到当前 FractalBot channel manager / handler 抽象

`callback` 相关实现与文档继续保留，但降级为：

- 第二阶段方案
- 对照参考
- 备用验证路径

## 2. 为什么现在改成 polling-first

### 2.1 用户已经明确要求

本次切换不是推测，而是用户的明确纠偏：

- **先做 polling**
- **callback 放第二期**

因此，KR2 的默认推进方向必须立即调整。

### 2.2 更贴近 `picoclaw` 已验证的现实路径

`https://github.com/sipeed/picoclaw` 已证明：

- 腾讯 iLink API + 二维码登录
- long polling (`GetUpdates`)
- cursor / state persistence

是一条真实存在、且更接近“个人微信/扫码登录/持续收消息”语义的路径。

### 2.3 callback-first 虽然更贴近当前 scaffold，但不再是默认业务优先级

此前选择 callback-first 的原因是：

- 当前 repo 里已有 `wechat` callback scaffold
- 更容易快速产出 listener / GET / POST 的运行证据

但用户已经明确希望优先验证 polling 方向，因此：

- callback-first 不再是默认主路径
- 这些已有产物只作为“第二阶段备选”保留

## 3. KR2 当前新的第一阶段目标

### Phase 1: polling-first 最小设计闭环
- 明确 `picoclaw` 路线里哪些结构可以映射到当前运行时
- 定义最小登录态 / token / baseUrl / cursor / state 模型
- 定义 polling loop 如何挂接到 FractalBot channel manager

### Phase 2: polling-first 本地验证
- 给出本地最小配置草案
- 给出轮询启动命令 / 模拟输入 / 状态证据模板
- 补第一次 polling proof

### Phase 3: callback 作为第二期
- 仅当需要补官方 callback 语义或做架构对照时再推进

## 4. 对已有 callback 文档的处理原则

以下内容不删除，但默认降级：

- `wechat-kr2-local-validation-checklist.md`
- `wechat-kr2-evidence-template.md`
- `wechat-local-config-overlay.example.yaml`
- `wechat-kr2-local-proof-commands.sh`

它们当前的定位改为：

- callback 第二期材料
- 备用验证路径
- 架构对照资料

## 5. 下一步

1. 新增 polling-first 的最小设计说明
2. 新增 polling-first 的本地验证 checklist
3. 更新 Memory Bank / heartbeat 状态，避免继续误报 callback-first
