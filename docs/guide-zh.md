# FractalBot 中文快速上手指南

## 概述

FractalBot 是一个用 Go 编写的**多渠道 AI Agent 网关**，支持 Telegram、Slack、Discord、飞书（Feishu/Lark）等即时通讯平台。你可以通过它把消息路由到后端的 Agent（如 Claude Code），实现「手机发消息 → Agent 自动干活 → 手机收结果」的工作流。

**核心特性：**

- 本地优先 — 跑在你自己的机器上，完全可控
- 多 Agent — 可同时管理多个 Agent（qa-1、coder-a 等），按需分配任务
- 多渠道 — Telegram / Slack / Discord / 飞书，一套配置搞定
- 安全白名单 — 仅允许指定用户与 Bot 交互

## 架构

```
            你的手机/电脑
               │
    ┌──────────┴──────────┐
    │  Telegram / Slack   │
    │  Discord  / 飞书    │
    └──────────┬──────────┘
               │ (消息)
    ┌──────────▼──────────┐
    │   FractalBot 网关   │
    │   (Go WebSocket)    │
    │   127.0.0.1:18789   │
    └──────────┬──────────┘
               │ (路由)
    ┌──────────▼──────────┐
    │   agent-manager     │
    │  (tmux + Claude)    │
    │                     │
    │  ┌───┐ ┌───┐ ┌───┐ │
    │  │qa │ │dev│ │...│  │
    │  └───┘ └───┘ └───┘  │
    └─────────────────────┘
```

## 环境准备

| 依赖 | 版本要求 | 用途 |
|------|---------|------|
| Go | 1.23+ | 编译 FractalBot |
| git | 任意 | 克隆仓库 |
| python3 | 3.10+ | agent-manager 脚本 |
| tmux | 任意 | Agent 会话管理 |

macOS 快速安装依赖：

```bash
brew install go git python3 tmux
```

Ubuntu/Debian：

```bash
sudo apt update && sudo apt install -y golang git python3 tmux
```

## 快速安装

### 方式一：一键脚本（推荐）

```bash
curl -fsSL https://raw.githubusercontent.com/fractalmind-ai/fractalbot/main/install.sh | bash
```

安装完成后，二进制文件位于 `~/.local/bin/fractalbot`，默认配置在 `~/.config/fractalbot/config.yaml`。

> 如需指定版本，设置环境变量：
> ```bash
> FRACTALBOT_REF=main curl -fsSL .../install.sh | bash
> ```

Linux 用户可追加 `--systemd-user` 自动注册为 systemd 用户服务：

```bash
curl -fsSL https://raw.githubusercontent.com/fractalmind-ai/fractalbot/main/install.sh | bash -s -- --systemd-user
```

### 方式二：手动编译

```bash
git clone https://github.com/fractalmind-ai/fractalbot.git
cd fractalbot
go build -o fractalbot ./cmd/fractalbot

# 验证
./fractalbot --help
```

## 配置 config.yaml

从模板开始：

```bash
cp config.example.yaml config.yaml
```

下面按渠道分别说明**必填**字段。

### Telegram（Polling 模式，本地开发推荐）

1. 在 Telegram 找 [@BotFather](https://t.me/BotFather)，发送 `/newbot` 创建一个 Bot，获取 `botToken`
2. 发送 `/mybots` → 选你的 Bot → Bot Settings → Group Privacy → **Turn off**（可选，仅 DM 模式可跳过）
3. 把 token 和你的 Telegram User ID 填入配置：

```yaml
channels:
  telegram:
    enabled: true
    botToken: "123456:ABC-DEF..."       # 从 @BotFather 获取
    adminID: 5088760910                  # 你的 Telegram User ID
    allowedUsers:
      - 5088760910                       # 白名单，同 adminID
    mode: "polling"                      # 本地推荐 polling
    pollingTimeoutSeconds: 25
    pollingLimit: 100
    pollingOffsetFile: "./workspace/telegram.offset"
```

> 不知道自己的 User ID？先随便填，启动后给 Bot 发 `/whoami`，Bot 会回复你的 ID。

### Slack（Socket Mode，无需公网端口）

1. 前往 [Slack API](https://api.slack.com/apps) 创建 App
2. 开启 **Socket Mode**，获取 `xapp-` 开头的 App-Level Token
3. 在 OAuth & Permissions 添加 Bot Token Scopes：`chat:write`、`im:history`、`im:read`
4. 安装 App 到 Workspace，获取 `xoxb-` 开头的 Bot Token
5. 在 Event Subscriptions 订阅 `message.im` 事件

```yaml
channels:
  slack:
    enabled: true
    botToken: "xoxb-your-bot-token"
    appToken: "xapp-your-app-token"
    allowedUsers:
      - "U08C93FU222"                    # Slack User ID
```

> Slack User ID 可在 Slack DM 中发 `/whoami` 给 Bot 获取。

### Discord

1. 前往 [Discord Developer Portal](https://discord.com/developers/applications) 创建 Application
2. 进入 Bot 页面，点击 Reset Token 获取 token
3. 开启 **Message Content Intent**
4. 用 OAuth2 URL Generator 邀请 Bot 到你的服务器（Scopes: `bot`；Permissions: `Send Messages`）

```yaml
channels:
  discord:
    enabled: true
    token: "your-bot-token"
    allowedUsers:
      - "123456789012345678"             # Discord User ID
```

### 飞书/Lark

1. 前往[飞书开放平台](https://open.feishu.cn/)创建应用
2. 获取 App ID 和 App Secret

```yaml
channels:
  feishu:
    enabled: true
    appId: "cli_xxx"
    appSecret: "your_app_secret"
    domain: "feishu"                     # 国际版用 "lark"
    allowedUsers:
      - "ou_xxxxx"
```

## 对接 agent-manager

FractalBot 通过 `agents.ohMyCode` 配置将消息路由到 [agent-manager](https://github.com/fractalmind-ai/agent-manager-skill)（基于 tmux + Claude Code 的 Agent 管理系统）。每个 Agent 跑在独立的 tmux session 里，收到消息后自动执行任务。

### 第一步：安装 agent-manager skill

在你的工作仓库中安装：

```bash
cd /path/to/your/workspace
npx openskills install agent-manager
```

安装后会出现 `.claude/skills/agent-manager/` 目录。

### 第二步：创建 Agent 定义文件

Agent 定义在 `agents/EMP_*.md` 文件中，使用 YAML frontmatter 配置。你需要**至少创建一个 Agent** 才能让 FractalBot 路由消息。

```bash
mkdir -p agents
```

但在创建 Agent 之前，你需要先配置根目录的 `AGENTS.md` — 这是 **main Agent**（协调者）的定义文件，也是所有 Agent 共享的工作规范。

#### 配置 AGENTS.md（main Agent + 工作规范）

`AGENTS.md` 有两个作用：
1. **YAML frontmatter** — 定义 main Agent 本身的配置（launcher、skills、心跳等）
2. **Markdown 正文** — 所有 Agent 共享的工作规范（安全规则、记忆管理、行为准则等）

在仓库根目录创建 `AGENTS.md`：

```markdown
---
name: main
description: Main
enabled: true
working_directory: ${REPO_ROOT}
launcher: claude
launcher_args: []
heartbeat:
  cron: "*/10 * * * *"
  max_runtime: 8m
  session_mode: auto
  enabled: true
skills:
  - agent-manager
  - use-fractalbot
---

# AGENTS.md - 工作规范

## 安全规则
- 不外泄私有数据
- 破坏性命令需先确认
- trash > rm（可恢复优先）

## 记忆管理
- 每日记录：memory/YYYY-MM-DD.md
- 长期记忆：MEMORY.md（仅主 session 加载）
- 重要决策和经验及时写入文件

## Agent 协作
- main 是协调者，优先分配任务给其他 Agent
- 开发任务 → dev/coder Agent
- 测试审查 → qa Agent
- 运维部署 → sre Agent
```

**main Agent frontmatter 关键字段：**

| 字段 | 说明 |
|------|------|
| `name: main` | 固定为 `main`，FractalBot 的 `defaultAgent` 通常指向它 |
| `heartbeat.cron` | 心跳定时任务（cron 表达式），main Agent 定期巡检 |
| `heartbeat.enabled` | 是否开启心跳 |
| `skills` | 必须包含 `agent-manager`（管理其他 Agent）和 `use-fractalbot`（回复消息） |

> main Agent 负责接收消息、分配任务给其他 Agent、定期巡检系统状态。
> 它是整个多 Agent 架构的「入口」。

#### 创建子 Agent（EMP_*.md）

创建你的第一个子 Agent，例如 `agents/EMP_0001.md`：

```markdown
---
name: dev
role: developer
description: "dev — 开发 Agent，负责编码和 bug 修复"
working_directory: ${REPO_ROOT}
launcher: claude
launcher_args: []
skills:
  - agent-manager
---

# DEV AGENT

## Primary responsibilities
- 实现功能开发和 bug 修复
- 保持变更小、可审查、易验证

## Operating rules
- 遵循 AGENTS.md 规范
- 优先通过 PR 提交，方便回滚
```

**frontmatter 字段说明：**

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | Agent 标识符，FractalBot 路由时使用此名称 |
| `role` | 否 | 角色描述（developer / qa / researcher 等） |
| `description` | 否 | 一行描述，`/agents` 命令时展示 |
| `working_directory` | 是 | 工作目录，`${REPO_ROOT}` 会替换为仓库根目录 |
| `launcher` | 是 | 启动器：`claude`（Claude Code）、`codex`（OpenAI Codex）或完整路径 |
| `launcher_args` | 否 | 启动器参数列表 |
| `skills` | 否 | 注入的 skill 名称列表 |
| `enabled` | 否 | 是否可启动，默认 `true`，设 `false` 禁用 |

**常见 launcher 配置：**

```yaml
# Claude Code
launcher: claude
launcher_args: []

# OpenAI Codex CLI
launcher: codex
launcher_args:
  - --model=gpt-5.3-codex
```

你可以创建多个 Agent（EMP_0002.md、EMP_0003.md…），分别负责不同职责：

```
agents/
├── EMP_0001.md    # dev — 开发
├── EMP_0002.md    # qa — 测试审查
└── EMP_0003.md    # sre — 运维部署
```

### 第三步：验证 Agent 可用

```bash
# 列出所有 Agent
python3 .claude/skills/agent-manager/scripts/main.py list

# 启动 Agent
python3 .claude/skills/agent-manager/scripts/main.py start dev

# 手动分配任务测试
python3 .claude/skills/agent-manager/scripts/main.py assign dev <<EOF
说 hello world
EOF

# 查看输出
python3 .claude/skills/agent-manager/scripts/main.py monitor dev
```

Agent 启动后会运行在 tmux session `agent-dev` 中，可以用 `tmux attach -t agent-dev` 直接查看。

### 第四步：配置 FractalBot 路由

在 `config.yaml` 中将消息路由到 agent-manager：

```yaml
agents:
  workspace: ./workspace
  maxConcurrent: 4

  ohMyCode:
    enabled: true
    workspace: "/path/to/your/workspace"     # 包含 agents/ 目录的仓库路径
    defaultAgent: "dev"                      # 普通消息默认路由到这个 Agent
    allowedAgents:                           # /agent 命令可选的 Agent 列表
      - "dev"
      - "qa"
    assignTimeoutSeconds: 90                 # Agent 响应超时（秒）
```

> `defaultAgent` 的值必须与某个 `agents/EMP_*.md` 中的 `name` 字段匹配。

### 第五步：安装 use-fractalbot skill

Agent 需要 [use-fractalbot](https://github.com/fractalmind-ai/use-fractalbot-skill) skill 才能通过 FractalBot 回复消息：

```bash
cd /path/to/your/workspace
npx openskills install use-fractalbot
# 验证
ls .claude/skills/use-fractalbot/SKILL.md
```

同时确保 Agent 定义文件的 `skills` 列表中包含 `use-fractalbot`：

```yaml
skills:
  - agent-manager
  - use-fractalbot    # 添加这一行
```

## 启动与验证

### 启动

```bash
# 前台运行（开发调试）
./fractalbot --config ./config.yaml

# 带详细日志
./fractalbot --config ./config.yaml --verbose

# 或在 tmux 中后台运行
tmux new-session -d -s fractalbot './fractalbot --config config.yaml'
```

### 验证

启动后，向 Bot 发送以下命令测试：

| 步骤 | 发送 | 预期结果 |
|------|------|----------|
| 1 | `/whoami` | 回复你的 User ID 等信息 |
| 2 | `/ping` | 回复 `pong` |
| 3 | `/agents` | 列出可用 Agent 名称 |
| 4 | `Hello` | Agent 接收任务并回复 |
| 5 | `/agent coder-a 写个 hello world` | 指定 Agent 执行任务 |

### HTTP 健康检查

```bash
# 健康检查
curl -s http://127.0.0.1:18789/health

# 查看状态
curl -s http://127.0.0.1:18789/status | python3 -m json.tool
```

## 常用命令速查

| 命令 | 说明 | 权限 |
|------|------|------|
| `/whoami` | 查看你的用户 ID | 所有人 |
| `/ping` | 健康检查 | 所有人 |
| `/agents` | 列出可用 Agent | 所有人 |
| `/agent <name> <task>` | 指定 Agent 执行任务 | 所有人 |
| `/monitor <name> [lines]` | 查看 Agent 最近输出（最多 200 行） | 所有人 |
| `/startagent <name>` | 启动指定 Agent | 仅 Admin |
| `/stopagent <name>` | 停止指定 Agent | 仅 Admin |
| `/doctor` | 运行诊断检查 | 仅 Admin |
| `tool echo <text>` | 内置 tool runtime 回声测试 | 所有人 |

> 直接发普通消息（不带 `/` 前缀）会路由到 `defaultAgent`。

## FAQ

### Q: 连接报 `connection refused`

**原因：** 网关未运行或端口不对。

```bash
# 检查进程
ps aux | grep fractalbot
# 检查端口
lsof -i :18789
# 确认配置中的 bind 和 port
grep -A2 'gateway:' config.yaml
```

### Q: Telegram 报 `unauthorized`

**原因：** botToken 无效或已过期。

1. 回到 @BotFather，发送 `/mybots`，确认 token 是否正确
2. 如果 token 泄露，点击 Revoke Token 重新生成
3. 更新 `config.yaml` 中的 `botToken`，重启

### Q: 发消息后 Agent 超时无响应

**原因：** agent-manager 未运行或 Agent 会话不存在。

```bash
# 检查 agent-manager 是否在运行
tmux ls

# 手动启动 Agent（在 oh-my-code 仓库）
python3 .claude/skills/agent-manager/scripts/main.py start qa-1

# 查看 Agent 日志
tmux attach -t qa-1
```

也可通过 Bot 命令诊断：
- `/doctor` — 运行诊断检查
- `/monitor qa-1` — 查看 Agent 最近输出

### Q: Slack 报 `channel "slack" not found`

**原因：** Slack 渠道未在配置中启用。

确认 `config.yaml` 中 `channels.slack.enabled: true`，且 `botToken` 和 `appToken` 均已填写。

### Q: 飞书/Discord 配置后没反应

1. 确认对应渠道 `enabled: true`
2. 确认 `allowedUsers` 中包含你的用户 ID（先用 `/whoami` 查询）
3. 确认 Bot 已正确邀请到工作区/服务器
4. 查看 FractalBot 终端日志排查

## 从 OpenClaw 迁移

如果你之前用的是 [OpenClaw/Clawdbot](https://github.com/clawdbot/clawdbot)（Node.js），以下是主要区别：

| 对比项 | OpenClaw (Clawdbot) | FractalBot |
|--------|--------------------|-----------:|
| 语言 | Node.js / TypeScript | Go |
| 安装 | `npm install` | `go build` 或一键脚本 |
| 配置格式 | `.env` + JSON | `config.yaml` |
| Telegram 模式 | Webhook | Polling（默认）/ Webhook |
| Agent 管理 | 内置 | 外部 agent-manager (tmux) |
| 渠道支持 | Telegram / Slack | Telegram / Slack / Discord / 飞书 |
| 多 Agent | 单 Agent | 多 Agent 并发 |

### 迁移步骤

1. **安装 FractalBot**（见上方「快速安装」）

2. **迁移配置** — 把 `.env` 中的 token 搬到 `config.yaml`：
   ```
   # 旧 (.env)
   TELEGRAM_BOT_TOKEN=123456:ABC

   # 新 (config.yaml)
   channels.telegram.botToken: "123456:ABC"
   ```

3. **设置 agent-manager** — FractalBot 依赖外部 agent-manager 管理 Claude 会话：
   ```bash
   # 在你的工作仓库中安装 agent-manager skill
   npx openskills install agent-manager
   ```

4. **安装 [use-fractalbot](https://github.com/fractalmind-ai/use-fractalbot-skill) skill** — Agent 需要此 skill 来回复消息：
   ```bash
   npx openskills install use-fractalbot
   ```

5. **启动 FractalBot** 并测试：
   ```bash
   ./fractalbot --config config.yaml
   # 发送 /ping 和 /whoami 验证
   ```

6. **停用旧服务** — 确认 FractalBot 工作正常后，停止 OpenClaw 进程

## 更多资源

- [GitHub 仓库](https://github.com/fractalmind-ai/fractalbot)
- [config.example.yaml](https://github.com/fractalmind-ai/fractalbot/blob/main/config.example.yaml) — 完整配置参考
- [agent-manager skill](https://github.com/fractalmind-ai/agent-manager-skill) — Agent 生命周期管理
- [use-fractalbot skill](https://github.com/fractalmind-ai/use-fractalbot-skill) — Agent 回复消息的 skill
- [ROADMAP.md](https://github.com/fractalmind-ai/fractalbot/blob/main/ROADMAP.md) — 开发路线图
- [CONTRIBUTING.md](https://github.com/fractalmind-ai/fractalbot/blob/main/CONTRIBUTING.md) — 贡献指南
