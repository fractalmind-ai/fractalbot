package channels

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

// DiscordBot implements a minimal Discord channel skeleton.
type DiscordBot struct {
	token string

	allowlist    DiscordAllowlist
	defaultAgent string
	agentAllow   AgentAllowlist

	handler IncomingMessageHandler

	session *discordgo.Session

	startFn       func(ctx context.Context) error
	stopFn        func() error
	sendMessageFn func(ctx context.Context, channelID, text string) error

	runningMu sync.RWMutex
	running   bool

	ctx    context.Context
	cancel context.CancelFunc

	telemetryMu  sync.RWMutex
	lastActivity time.Time
	lastError    time.Time
}

func NewDiscordBot(token string, allowedUsers []string, defaultAgent string, allowedAgents []string) (*DiscordBot, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, errors.New("discord token is required")
	}

	return &DiscordBot{
		token:        trimmed,
		allowlist:    NewDiscordAllowlist(allowedUsers),
		defaultAgent: strings.TrimSpace(defaultAgent),
		agentAllow:   NewAgentAllowlist(allowedAgents),
		ctx:          context.Background(),
	}, nil
}

func (b *DiscordBot) Name() string {
	return "discord"
}

func (b *DiscordBot) SetHandler(handler IncomingMessageHandler) {
	b.handler = handler
}

func (b *DiscordBot) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

// LastActivity reports the last time the bot saw a message or successfully sent one.
func (b *DiscordBot) LastActivity() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastActivity
}

// LastError reports the last time the bot encountered a channel error.
func (b *DiscordBot) LastError() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastError
}

func (b *DiscordBot) markActivity() {
	b.telemetryMu.Lock()
	b.lastActivity = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *DiscordBot) markError() {
	b.telemetryMu.Lock()
	b.lastError = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *DiscordBot) setRunning(running bool) {
	b.runningMu.Lock()
	b.running = running
	b.runningMu.Unlock()
}

func (b *DiscordBot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	if b.startFn == nil {
		if err := b.initClients(); err != nil {
			return err
		}
	}

	if b.startFn == nil {
		return errors.New("discord start function not configured")
	}

	if err := b.startFn(b.ctx); err != nil {
		return err
	}

	b.setRunning(true)
	return nil
}

func (b *DiscordBot) Stop() error {
	if b.cancel != nil {
		b.cancel()
	}
	if b.stopFn != nil {
		if err := b.stopFn(); err != nil {
			return err
		}
	}
	b.setRunning(false)
	return nil
}

func (b *DiscordBot) SendMessage(ctx context.Context, chatID int64, text string) error {
	_ = ctx
	_ = chatID
	_ = text
	return errors.New("discord SendMessage requires a string channel ID")
}

func (b *DiscordBot) initClients() error {
	if b.sendMessageFn != nil && b.startFn != nil {
		return nil
	}
	if strings.TrimSpace(b.token) == "" {
		return errors.New("discord token is required")
	}

	session, err := discordgo.New("Bot " + b.token)
	if err != nil {
		return err
	}
	session.Identify.Intents = discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent

	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		msg := discordMessageFromEvent(m)
		if msg == nil {
			return
		}
		ctx := b.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		b.handleMessageEvent(ctx, msg)
	})

	b.session = session
	b.sendMessageFn = b.sendText
	b.startFn = b.startGateway
	b.stopFn = b.stopGateway
	return nil
}

func (b *DiscordBot) startGateway(ctx context.Context) error {
	if b.session == nil {
		return errors.New("discord session not initialized")
	}
	go func() {
		<-ctx.Done()
		_ = b.session.Close()
	}()
	return b.session.Open()
}

func (b *DiscordBot) stopGateway() error {
	if b.session != nil {
		return b.session.Close()
	}
	return nil
}

func (b *DiscordBot) handleMessageEvent(ctx context.Context, msg *discordInboundMessage) {
	if msg == nil {
		return
	}
	if msg.channelType != "dm" {
		return
	}

	b.markActivity()

	if isDiscordSafeCommand(msg.text) {
		if handled, cmdErr := b.handleCommand(ctx, msg); handled {
			if cmdErr != nil {
				_ = b.reply(ctx, msg, fmt.Sprintf("❌ %v", cmdErr))
			}
			return
		}
	}

	if !b.allowlist.Allowed(msg.userID) {
		_ = b.reply(ctx, msg, "❌ Unauthorized. Ask an admin to add your Discord user ID to channels.discord.allowedUsers.\nTip: use /whoami to get your user ID.")
		return
	}

	if handled, cmdErr := b.handleCommand(ctx, msg); handled {
		if cmdErr != nil {
			reply := fmt.Sprintf("❌ %v", cmdErr)
			if isAgentNotAllowedError(cmdErr) {
				reply = agentNotAllowedMessage(cmdErr, b.defaultAgent, b.agentAllow)
			} else if isAgentAllowlistError(cmdErr) {
				reply = fmt.Sprintf("%s\nTip: use /agents to see allowed agents.", reply)
			}
			_ = b.reply(ctx, msg, reply)
		}
		return
	}

	selection, err := ParseAgentSelection(msg.text)
	if err != nil {
		reply := fmt.Sprintf("❌ %v", err)
		if isAgentNotAllowedError(err) {
			reply = agentNotAllowedMessage(err, b.defaultAgent, b.agentAllow)
		} else if isAgentAllowlistError(err) {
			reply = fmt.Sprintf("%s\nTip: use /agents to see allowed agents.", reply)
		}
		_ = b.reply(ctx, msg, reply)
		return
	}

	if strings.TrimSpace(selection.Task) == "" {
		return
	}

	enforceSelection := selection.Specified || b.defaultAgent != "" || b.agentAllow.configured
	if enforceSelection {
		selection, err = ResolveAgentSelection(selection, b.defaultAgent, b.agentAllow)
		if err != nil {
			reply := fmt.Sprintf("❌ %v", err)
			if isDefaultAgentMissingError(err) && !selection.Specified && b.agentAllow.configured {
				reply = "❌ Default agent is not configured.\nTip: use /agent <name> <task> or set agents.ohMyCode.defaultAgent."
			} else if isAgentNotAllowedError(err) {
				reply = agentNotAllowedMessage(err, b.defaultAgent, b.agentAllow)
			} else if isAgentAllowlistError(err) {
				reply = fmt.Sprintf("%s\nTip: use /agents to see allowed agents.", reply)
			}
			_ = b.reply(ctx, msg, reply)
			return
		}
	}

	if b.handler != nil {
		replyText, err := b.handler.HandleIncoming(ctx, b.toProtocolMessage(msg, selection.Task, selection.Agent))
		if err != nil {
			log.Printf("discord handler error: %v", err)
			replyText = "❌ Something went wrong. Please try again."
		}
		if strings.TrimSpace(replyText) != "" {
			_ = b.reply(ctx, msg, replyText)
		}
		return
	}

	if selection.Task != "" {
		_ = b.reply(ctx, msg, fmt.Sprintf("echo: %s", selection.Task))
	}
}

func (b *DiscordBot) handleCommand(ctx context.Context, msg *discordInboundMessage) (bool, error) {
	text := strings.TrimSpace(msg.text)
	if text == "" || !strings.HasPrefix(text, "/") {
		return false, nil
	}

	fields := strings.Fields(text)
	if len(fields) == 0 {
		return true, nil
	}

	command := fields[0]
	if command == "/agent" {
		return false, nil
	}

	switch command {
	case "/help", "/start":
		return true, b.reply(ctx, msg, b.helpText())
	case "/agents":
		names := b.agentAllow.Names()
		if len(names) == 0 {
			if trimmed := strings.TrimSpace(b.defaultAgent); trimmed != "" {
				names = []string{trimmed}
			}
		}
		if len(names) == 0 {
			return true, b.reply(ctx, msg, noAgentsConfiguredMessage)
		}
		var sb strings.Builder
		sb.WriteString("Allowed agents:\n")
		if trimmed := strings.TrimSpace(b.defaultAgent); trimmed != "" {
			sb.WriteString(fmt.Sprintf("Default agent: %s\n", trimmed))
		}
		for _, name := range names {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		return true, b.reply(ctx, msg, strings.TrimSpace(sb.String()))
	case "/whoami":
		reply := fmt.Sprintf("user_id: %s\nchannel_id: %s", msg.userID, msg.channelID)
		return true, b.reply(ctx, msg, reply)
	default:
		return true, fmt.Errorf("unknown command: %s", command)
	}
}

func (b *DiscordBot) helpText() string {
	lines := []string{
		"FractalBot Discord Help",
		"",
		"Commands:",
		"  /help - show this help",
		"  /agents - list allowed agents",
		"  /whoami - show your Discord IDs",
		"",
		"Agent routing:",
		"  /agent <name> <task...>",
	}
	return strings.Join(lines, "\n")
}

func isDiscordSafeCommand(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "/help", "/start", "/whoami":
		return true
	default:
		return false
	}
}

func (b *DiscordBot) reply(ctx context.Context, msg *discordInboundMessage, text string) error {
	if b.sendMessageFn == nil {
		return errors.New("discord sender not configured")
	}
	if err := b.sendMessageFn(ctx, msg.channelID, TruncateDiscordReply(text)); err != nil {
		b.markError()
		return err
	}
	b.markActivity()
	return nil
}

func (b *DiscordBot) sendText(ctx context.Context, channelID, text string) error {
	if b.session == nil {
		b.markError()
		return errors.New("discord session not initialized")
	}
	if strings.TrimSpace(channelID) == "" {
		b.markError()
		return errors.New("discord channel ID is required")
	}
	_, err := b.session.ChannelMessageSend(channelID, text)
	if err != nil {
		b.markError()
		return err
	}
	_ = ctx
	return nil
}

func (b *DiscordBot) toProtocolMessage(msg *discordInboundMessage, text, agent string) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel":    "discord",
			"text":       text,
			"agent":      agent,
			"user_id":    msg.userID,
			"channel_id": msg.channelID,
			"chatType":   msg.channelType,
		},
	}
}

type discordInboundMessage struct {
	text        string
	userID      string
	channelID   string
	channelType string
}

func discordMessageFromEvent(event *discordgo.MessageCreate) *discordInboundMessage {
	if event == nil || event.Author == nil || event.Message == nil {
		return nil
	}
	if event.Author.Bot {
		return nil
	}
	channelType := "dm"
	if strings.TrimSpace(event.GuildID) != "" {
		channelType = "guild"
	}
	if strings.TrimSpace(event.ChannelID) == "" || strings.TrimSpace(event.Author.ID) == "" {
		return nil
	}
	return &discordInboundMessage{
		text:        event.Content,
		userID:      event.Author.ID,
		channelID:   event.ChannelID,
		channelType: channelType,
	}
}

type DiscordAllowlist struct {
	configured bool
	allowed    map[string]struct{}
}

func NewDiscordAllowlist(values []string) DiscordAllowlist {
	allowed := make(map[string]struct{})
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	return DiscordAllowlist{configured: len(allowed) > 0, allowed: allowed}
}

func (a DiscordAllowlist) Allowed(userID string) bool {
	if !a.configured {
		return false
	}
	if userID == "" {
		return false
	}
	_, ok := a.allowed[userID]
	return ok
}
