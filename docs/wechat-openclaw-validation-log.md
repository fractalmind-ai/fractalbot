# 微信 OpenClaw 官方插件验证记录

> Status: in progress
> Updated: 2026-03-23 (UTC+8)
> Goal: 留下本轮错误路径与纠偏证据，避免后续再次把“安装 OpenClaw”当成默认动作。

## 0. 纠偏说明（2026-03-23）

用户已明确说明：

- 当前环境是 **`https://github.com/fractalmind-ai` 驱动运行**
- **不要再安装 OpenClaw**
- 之前已经卸载过 OpenClaw

因此，本文档中的 3 次尝试应视为：

- **一次错误路径的排查记录**
- 不是后续 heartbeat 应继续复现的默认步骤

后续验证必须改为：

1. 先确认 fractalmind-ai 当前运行时中，微信能力应接入的真实宿主
2. 只把官方 `openclaw-weixin` 包作为能力边界/兼容性参考
3. 不再继续执行新的 OpenClaw 安装或升级动作

## 1. 本地环境基线

- `openclaw --version` → `2026.2.6-3`
- `npm view openclaw version` → `2026.3.13`
- `node -v` → `v22.21.0`
- `npm -v` → `10.9.4`

初步判断：本地 OpenClaw 版本**落后于 npm 最新版**，存在插件 SDK 兼容性风险。

## 2. 尝试记录

### 尝试 1：官方一键安装
命令：

```bash
npx -y @tencent-weixin/openclaw-weixin-cli install
```

结果：
- 成功下载并安装插件到 `~/.openclaw/extensions/openclaw-weixin`
- 安装阶段出现安全提示：
  - `WARNING: Plugin "openclaw-weixin" contains dangerous code patterns: Environment variable access combined with network send`
- 安装后自动首次连接失败：
  - `TypeError: (0 , _pluginSdk.resolvePreferredOpenClawTmpDir) is not a function`
  - `Channel login failed: Error: Unsupported channel: openclaw-weixin`

### 尝试 2：检查插件加载状态
命令：

```bash
openclaw plugins list
```

结果：
- `@tencent-weixin/openclaw-weixin` 已出现在插件列表中
- 状态为 `error`
- 关键报错：

```text
openclaw-weixin failed to load ... TypeError: (0 , _pluginSdk.resolvePreferredOpenClawTmpDir) is not a function
```

### 尝试 3：手动登录 channel
命令：

```bash
openclaw channels login --channel openclaw-weixin
```

结果：
- 失败
- 报错：

```text
Channel login failed: Error: Unsupported channel: openclaw-weixin
```

## 3. 当前结论

当前已经确认：

1. **官方插件安装成功，但加载失败**
2. 失败点出现在插件运行期，而不是 npm 下载/解压阶段
3. 错误特征强烈指向 **OpenClaw core 与插件 SDK 的版本不兼容**
   - 插件调用了 `resolvePreferredOpenClawTmpDir`
   - 当前本地 OpenClaw 版本未提供该函数

## 4. 对 O7 的意义

- 这 3 次尝试证明：**把 OpenClaw 当作当前环境宿主是有偏差的**
- 本轮更重要的产出不是兼容性结论本身，而是确认了需要按用户约束收敛路线
- 后续 KR2 必须切换到：**在 fractalmind-ai 当前运行时内验证微信能力，而不是继续围绕 OpenClaw 安装/升级打转**

## 5. 下一步建议

优先级顺序：

1. 先确认当前 `fractalmind-ai` 驱动运行时中，微信能力应挂接的真实入口
2. 把官方 `openclaw-weixin` 包保留为能力边界与兼容性参考，而不是默认执行路径
3. 基于当前运行时重新设计 KR2 的最小闭环验证步骤
4. 若后续仍需引用 upstream OpenClaw，只能作为例外场景单独说明，不能再默认执行安装/升级

## 6. 当前不做的事

在用户重新明确允许之前，**不再执行新的 OpenClaw 安装/升级动作，也不继续扩大 FractalBot 自研 wechat callback 的实现范围**。
