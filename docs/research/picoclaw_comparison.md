# picoclaw 对 FractalBot 精简后的借鉴

**调研日期**: 2026-03-03
**调研对象**: https://github.com/sipeed/picoclaw (21.8K stars, Go, MIT)
**对比对象**: fractalmind-ai/fractalbot (精简后纯 CLI Agent 消息网关)

---

## 1. picoclaw 概况

| 维度 | 详情 |
|------|------|
| 定位 | 超轻量 AI Agent 网关, <10MB 内存 |
| 语言 | Go 1.25 |
| Stars | 21,792 |
| Forks | 2,788 |
| Contributors | 20+ |
| 许可 | MIT |
| 消息通道 | 14 个 (Telegram/Discord/Slack/WhatsApp/Feishu/LINE/QQ/DingTalk/OneBot/WeCom/MaixCam/Pico) |
| 核心依赖 | discordgo, telego, slack-go, whatsmeow, cobra, zerolog, gorilla/websocket |

## 2. 架构对比

### 2.1 功能重叠

| 能力 | picoclaw | FractalBot (精简后) |
|------|----------|-------------------|
| 多通道消息适配 | 14 通道 | 5 通道 (TG/Slack/Discord/Feishu/iMessage) |
| 消息收发 API | WebSocket (Pico protocol) | HTTP REST + CLI |
| Agent 路由 | 7 级级联绑定 | allow-list + oh-my-code routing |
| 安全模型 | allow-list + OAuth + PKCE | 用户白名单 + 默认拒绝 |
| CLI | cobra-based | cobra-based |
| 配置 | JSON + env vars | YAML |

### 2.2 picoclaw 有而 FractalBot 没有

- **Channel 可选能力接口** (TypingCapable, MessageEditor, ReactionCapable, PlaceholderCapable, MediaSender)
- **自注册工厂模式** (init() + blank import, 零侵入新增通道)
- **In-process Message Bus** (3 管道: inbound/outbound/outboundMedia)
- **Per-channel worker + rate limiter** (按平台限速: TG 20/s, Discord 1/s)
- **分类重试策略** (permanent/rate-limit/transient)
- **Placeholder pipeline** (typing → reaction → edit placeholder, TTL janitor)
- **Session key 4 级 DM 隔离** + 跨平台身份链接
- **Skills 系统** (SKILL.md markdown-based, 3 级加载)
- **MCP 集成** (stdio/SSE/HTTP transports)
- **Native WebSocket protocol** (Pico channel)
- **System prompt caching** (mtime-based invalidation + Anthropic cache_control)

### 2.3 FractalBot 有而 picoclaw 没有

- **iMessage 适配** (macOS 专属)
- **oh-my-code Agent 路由** (assign + routing context 格式)
- **Gateway server** (独立 WebSocket + HTTP 网关模式)
- **YAML 配置** (picoclaw 用 JSON)

## 3. 可借鉴的设计 (按优先级)

### P0: Channel 接口拆分 — 可选能力接口

**picoclaw 做法**:

核心接口只保留必需方法:
```go
type Channel interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Send(ctx context.Context, msg bus.OutboundMessage) error
    IsRunning() bool
    IsAllowed(senderID string) bool
}
```

可选能力用独立 interface, 运行时 type assertion 发现:
```go
type TypingCapable interface {
    StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

type MessageEditor interface {
    EditMessage(ctx context.Context, chatID, messageID, content string) error
}

type ReactionCapable interface {
    AddReaction(ctx context.Context, chatID, messageID, emoji string) (undo func(), err error)
}

type PlaceholderCapable interface {
    SendPlaceholder(ctx context.Context, chatID, text string) (messageID string, err error)
}

type MediaSender interface {
    SendMedia(ctx context.Context, chatID string, parts []MediaPart) error
}
```

使用方:
```go
if tc, ok := channel.(TypingCapable); ok {
    stop, _ := tc.StartTyping(ctx, chatID)
    defer stop()
}
```

**FractalBot 建议**: 将当前单一 Adapter interface 拆分为 core + optional capabilities。

### P0: 自注册工厂模式

**picoclaw 做法**:
```go
// pkg/channels/registry.go
var registry = map[string]ChannelFactory{}

func RegisterFactory(name string, f ChannelFactory) {
    registry[name] = f
}

// pkg/channels/telegram/init.go
func init() {
    channels.RegisterFactory("telegram", NewTelegramChannel)
}

// cmd/picoclaw/internal/gateway/imports.go
import (
    _ "github.com/sipeed/picoclaw/pkg/channels/telegram"
    _ "github.com/sipeed/picoclaw/pkg/channels/discord"
    _ "github.com/sipeed/picoclaw/pkg/channels/slack"
)
```

**FractalBot 建议**: 替换现有硬编码的 channel 初始化, 使新增通道只需: 1) 实现包 2) 加一行 import。

### P1: In-Process Message Bus

**picoclaw 做法**:
```go
type MessageBus struct {
    inbound       chan InboundMessage       // 通道 → agent
    outbound      chan OutboundMessage       // agent → 通道
    outboundMedia chan OutboundMediaMessage   // agent → 通道 (附件)
    done          chan struct{}
    closed        atomic.Bool
}
```

3 管道分离, buffered channel (64), context-aware publish, atomic close。

**FractalBot 建议**: 引入简单 bus 解耦 channel adapter 和 agent router, 避免直接函数调用耦合。

### P1: Per-Channel Worker + Rate Limiter

**picoclaw 做法**:
- 每通道独立 goroutine + buffered message queue (16)
- 平台特定限速: Telegram 20 msg/s, Discord 1 msg/s, Slack 1 msg/s
- 错误分 3 类:
  - `ErrNotRunning` / `ErrSendFailed` → 不重试
  - `ErrRateLimit` → 固定 1s 重试
  - `ErrTemporary` → 指数退避 (500ms base, 8s max, 3 次)
- Placeholder pipeline: typing → reaction → edit placeholder → send
- TTL janitor: 每 10s 清理过期状态 (typing 5min, placeholder 10min)

**FractalBot 建议**: 当前直接 Send 可能被平台限速, 加入 worker + rate limiter 提高可靠性。

### P2: 路由级联

**picoclaw 做法**: 7 级优先级匹配:
1. `binding.peer` (精确 chatID)
2. `binding.peer.parent` (父级 peer)
3. `binding.guild` (Discord 服务器)
4. `binding.team` (Slack 团队)
5. `binding.account` (平台账号)
6. `binding.channel` (通道通配)
7. `default` (默认 agent)

Session key 支持 4 种隔离级别 + identity_links 跨平台身份合并。

**FractalBot 建议**: 当前 oh-my-code 路由已满足需求, 但如果未来多 Agent 场景可参考级联设计。

### P2: Skills Markdown 格式

**picoclaw 做法**:
- `SKILL.md`: YAML frontmatter (name, description) + markdown body
- 3 级加载: workspace > global (~/.picoclaw/skills) > builtin
- System prompt 只注入摘要, LLM 按需通过 read_file 读完整内容
- ClawHub 远程 registry 支持

**FractalBot 建议**: 我们已有 openskills 体系, 格式类似。可参考 picoclaw 的 3 级加载和摘要注入模式。

## 4. 不适合借鉴的

| 设计 | 原因 |
|------|------|
| LLM Provider 管理 (model_list, fallback chain) | FractalBot 不直接调 LLM, 只做消息转发 |
| Agent Loop (工具循环、上下文构建) | 不在网关范围内 |
| Memory 系统 (MEMORY.md, 日志) | Agent 自身负责, 不是网关功能 |
| Hardware I2C/SPI 工具 | 嵌入式设备专属 |
| TUI Launcher | 不需要图形启动器 |
| OAuth/PKCE 认证 | FractalBot 用的是 API key + 白名单, 更简单 |
| MCP 集成 | 网关不需要 tool 系统 |
| Pico WebSocket 协议 | FractalBot 已有 Gateway WebSocket |

## 5. 总结

picoclaw 是一个架构优雅的 Go 消息网关, 虽然定位更偏向嵌入式 AI 助手, 但其 **Channel 抽象层设计** (核心接口 + 可选能力 + 自注册工厂 + per-channel worker) 是业界最佳实践, 值得 FractalBot 精简后直接参考。

关键收获:
1. **接口拆分** 比单一大接口更灵活
2. **自注册工厂** 比硬编码注册更可扩展
3. **Message Bus 解耦** 比直接调用更健壮
4. **分类重试** 比统一重试更精准
5. **Rate limiter** 是生产级网关必备
