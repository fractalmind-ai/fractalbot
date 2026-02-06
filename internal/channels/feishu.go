package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const (
	feishuDomainFeishu = "feishu"
	feishuDomainLark   = "lark"
)

type FeishuBot struct {
	appID        string
	appSecret    string
	domain       string
	allowlist    FeishuAllowlist
	defaultAgent string
	agentAllow   AgentAllowlist

	handler IncomingMessageHandler

	apiClient *lark.Client
	wsClient  *ws.Client

	startFn       func(ctx context.Context) error
	stopFn        func() error
	sendMessageFn func(ctx context.Context, receiveIDType, receiveID, text string) error

	runningMu sync.RWMutex
	running   bool

	ctx    context.Context
	cancel context.CancelFunc

	telemetryMu  sync.RWMutex
	lastActivity time.Time
	lastError    time.Time
}

func NewFeishuBot(appID, appSecret, domain string, allowedUsers []string, defaultAgent string, allowedAgents []string) (*FeishuBot, error) {
	trimmedID := strings.TrimSpace(appID)
	trimmedSecret := strings.TrimSpace(appSecret)
	if trimmedID == "" || trimmedSecret == "" {
		return nil, errors.New("feishu appId/appSecret are required")
	}

	resolvedDomain := strings.ToLower(strings.TrimSpace(domain))
	if resolvedDomain == "" {
		resolvedDomain = feishuDomainFeishu
	}
	if resolvedDomain != feishuDomainFeishu && resolvedDomain != feishuDomainLark {
		return nil, fmt.Errorf("invalid feishu domain: %q", domain)
	}

	return &FeishuBot{
		appID:        trimmedID,
		appSecret:    trimmedSecret,
		domain:       resolvedDomain,
		allowlist:    NewFeishuAllowlist(allowedUsers),
		defaultAgent: strings.TrimSpace(defaultAgent),
		agentAllow:   NewAgentAllowlist(allowedAgents),
		ctx:          context.Background(),
	}, nil
}

func (b *FeishuBot) Name() string {
	return "feishu"
}

func (b *FeishuBot) SetHandler(handler IncomingMessageHandler) {
	b.handler = handler
}

func (b *FeishuBot) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

// LastActivity reports the last time the bot saw a message or successfully sent one.
func (b *FeishuBot) LastActivity() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastActivity
}

// LastError reports the last time the bot encountered a channel error.
func (b *FeishuBot) LastError() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastError
}

func (b *FeishuBot) markActivity() {
	b.telemetryMu.Lock()
	b.lastActivity = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *FeishuBot) markError() {
	b.telemetryMu.Lock()
	b.lastError = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *FeishuBot) setRunning(running bool) {
	b.runningMu.Lock()
	b.running = running
	b.runningMu.Unlock()
}

func (b *FeishuBot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	if b.startFn == nil {
		b.initClients()
	}

	if b.startFn == nil {
		return errors.New("feishu start function not configured")
	}

	if err := b.startFn(b.ctx); err != nil {
		return err
	}

	b.setRunning(true)
	return nil
}

func (b *FeishuBot) Stop() error {
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

func (b *FeishuBot) SendMessage(ctx context.Context, chatID int64, text string) error {
	_ = ctx
	_ = chatID
	_ = text
	return errors.New("feishu SendMessage requires a string receive_id")
}

func (b *FeishuBot) initClients() {
	if b.sendMessageFn != nil && b.startFn != nil {
		return
	}

	domainURL := resolveFeishuDomain(b.domain)
	b.apiClient = lark.NewClient(b.appID, b.appSecret, lark.WithOpenBaseUrl(domainURL))

	dispatcher := dispatcher.NewEventDispatcher("", "")
	dispatcher.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		return b.handleMessageEvent(ctx, event)
	})

	b.wsClient = ws.NewClient(
		b.appID,
		b.appSecret,
		ws.WithDomain(domainURL),
		ws.WithEventHandler(dispatcher),
	)

	b.sendMessageFn = b.sendText
	b.startFn = b.startLongConnection
}

func (b *FeishuBot) startLongConnection(ctx context.Context) error {
	if b.wsClient == nil {
		return errors.New("feishu websocket client not initialized")
	}

	go func() {
		if err := b.wsClient.Start(ctx); err != nil {
			log.Printf("feishu websocket error: %v", err)
		}
	}()
	return nil
}

func (b *FeishuBot) sendText(ctx context.Context, receiveIDType, receiveID, text string) error {
	if b.apiClient == nil {
		b.markError()
		return errors.New("feishu api client not initialized")
	}
	if strings.TrimSpace(receiveID) == "" {
		b.markError()
		return errors.New("feishu receive_id is required")
	}

	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		b.markError()
		return fmt.Errorf("failed to marshal feishu content: %w", err)
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType("text").
			Content(string(payload)).
			Build()).
		Build()

	resp, err := b.apiClient.Im.V1.Message.Create(ctx, req)
	if err != nil {
		b.markError()
		return err
	}
	if !resp.Success() {
		b.markError()
		return fmt.Errorf("feishu send failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	b.markActivity()
	return nil
}

func (b *FeishuBot) handleMessageEvent(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	msg, err := parseFeishuInbound(event)
	if err != nil {
		b.markError()
		log.Printf("feishu parse error: %v", err)
		return nil
	}
	if msg == nil {
		return nil
	}
	if msg.chatType != "p2p" {
		return nil
	}
	b.markActivity()

	if handled, cmdErr := b.handleCommand(ctx, msg); handled {
		if cmdErr != nil {
			_ = b.reply(ctx, msg, fmt.Sprintf("❌ %v", cmdErr))
		}
		return nil
	}

	if !b.allowlist.Allowed(msg.openID, msg.userID) {
		_ = b.reply(ctx, msg, fmt.Sprintf("❌ Unauthorized. open_id: %s, user_id: %s. Ask an admin to add your IDs to channels.feishu.allowedUsers.\nTip: use /whoami to get your IDs.", msg.openID, msg.userID))
		return nil
	}

	if isIncompleteFeishuAgentCommand(msg.text) {
		_ = b.reply(ctx, msg, "❌ usage: /agent <name> <task>\nTip: use /agents to see allowed agents.")
		return nil
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
		return nil
	}
	if strings.TrimSpace(selection.Task) == "" {
		return nil
	}

	enforceSelection := selection.Specified || b.defaultAgent != "" || b.agentAllow.configured
	if enforceSelection {
		selection, err = ResolveAgentSelection(selection, b.defaultAgent, b.agentAllow)
		if err != nil {
			reply := fmt.Sprintf("❌ %v", err)
			if !selection.Specified && (isDefaultAgentMissingError(err) || isInvalidAgentNameError(err)) {
				reply = "❌ Default agent is missing or invalid.\nSet agents.ohMyCode.defaultAgent or use /agent <name> <task>.\nTip: use /agents to see allowed agents."
			} else if isAgentNotAllowedError(err) {
				reply = agentNotAllowedMessage(err, b.defaultAgent, b.agentAllow)
			} else if isAgentAllowlistError(err) {
				reply = fmt.Sprintf("%s\nTip: use /agents to see allowed agents.", reply)
			}
			_ = b.reply(ctx, msg, reply)
			return nil
		}
	}

	if b.handler != nil {
		replyText, err := b.handler.HandleIncoming(ctx, b.toProtocolMessage(msg, selection.Task, selection.Agent))
		if err != nil {
			b.markError()
			log.Printf("feishu handler error: %v", err)
			replyText = "❌ Something went wrong. Please try again."
		}
		if strings.TrimSpace(replyText) != "" {
			_ = b.reply(ctx, msg, replyText)
		}
		return nil
	}

	_ = b.reply(ctx, msg, fmt.Sprintf("echo: %s", selection.Task))
	return nil
}

func (b *FeishuBot) handleCommand(ctx context.Context, msg *feishuInboundMessage) (bool, error) {
	text := strings.TrimSpace(msg.text)
	if text == "" || !strings.HasPrefix(text, "/") {
		return false, nil
	}

	parts := strings.Fields(text)
	if len(parts) == 0 {
		return true, nil
	}

	command := parts[0]
	if idx := strings.IndexByte(command, '@'); idx != -1 {
		command = command[:idx]
	}
	if command == "/agent" || command == "/to" {
		return false, nil
	}

	switch command {
	case "/help", "/start":
		return true, b.reply(ctx, msg, b.helpText())
	case "/status":
		return true, b.reply(ctx, msg, b.statusText())
	case "/agents":
		names := b.agentAllow.Names()
		defaultName := strings.TrimSpace(b.defaultAgent)
		if len(names) > 0 && defaultName != "" {
			names = filterOutAgentName(names, defaultName)
		}
		if len(names) == 0 && defaultName == "" {
			return true, b.reply(ctx, msg, noAgentsConfiguredMessage)
		}
		var sb strings.Builder
		sb.WriteString("Allowed agents:\n")
		if defaultName != "" {
			sb.WriteString(fmt.Sprintf("Default agent: %s\n", defaultName))
		}
		for _, name := range names {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		return true, b.reply(ctx, msg, strings.TrimSpace(sb.String()))
	case "/whoami":
		reply := fmt.Sprintf("open_id: %s\nuser_id: %s\nchat_id: %s", msg.openID, msg.userID, msg.chatID)
		return true, b.reply(ctx, msg, reply)
	default:
		return true, fmt.Errorf("unknown command: %s", command)
	}
}

func (b *FeishuBot) helpText() string {
	lines := []string{
		"FractalBot Feishu Help",
		"",
		"Commands:",
		"  /help - show this help",
		"  /status - bot status",
		"  /agents - list allowed agents",
		"  /whoami - show your Feishu IDs",
		"",
		"Agent routing:",
		"  /agent <name> <task...>",
		"  /to <name> <task...> (alias of /agent)",
	}
	return strings.Join(lines, "\n")
}

func (b *FeishuBot) statusText() string {
	lastActivity := "never"
	if ts := b.LastActivity(); !ts.IsZero() {
		lastActivity = ts.UTC().Format(time.RFC3339)
	}
	lastError := "none"
	if ts := b.LastError(); !ts.IsZero() {
		lastError = ts.UTC().Format(time.RFC3339)
	}
	defaultAgent := strings.TrimSpace(b.defaultAgent)
	if defaultAgent == "" {
		defaultAgent = "(none)"
	}
	allowlistConfigured := b.agentAllow.configured
	return strings.Join([]string{
		"Bot Status",
		"bot: feishu",
		fmt.Sprintf("running: %t", b.IsRunning()),
		fmt.Sprintf("last_activity: %s", lastActivity),
		fmt.Sprintf("last_error: %s", lastError),
		fmt.Sprintf("default_agent: %s", defaultAgent),
		fmt.Sprintf("allowed_agents_configured: %t", allowlistConfigured),
	}, "\n")
}

func (b *FeishuBot) reply(ctx context.Context, msg *feishuInboundMessage, text string) error {
	if b.sendMessageFn == nil {
		return errors.New("feishu sender not configured")
	}
	if err := b.sendMessageFn(ctx, msg.replyIDType, msg.replyID, TruncateFeishuReply(text)); err != nil {
		b.markError()
		return err
	}
	b.markActivity()
	return nil
}

func (b *FeishuBot) toProtocolMessage(msg *feishuInboundMessage, text, agent string) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel":  "feishu",
			"text":     text,
			"agent":    agent,
			"chat_id":  msg.chatID,
			"open_id":  msg.openID,
			"user_id":  msg.userID,
			"message":  msg.messageID,
			"chatType": msg.chatType,
		},
	}
}

type feishuInboundMessage struct {
	text        string
	openID      string
	userID      string
	chatID      string
	chatType    string
	messageID   string
	replyIDType string
	replyID     string
}

type feishuTextContent struct {
	Text string `json:"text"`
}

func parseFeishuInbound(event *larkim.P2MessageReceiveV1) (*feishuInboundMessage, error) {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil || event.Event.Sender.SenderId == nil {
		return nil, nil
	}
	msg := event.Event.Message
	if msg.MessageType == nil || *msg.MessageType != "text" {
		return nil, nil
	}
	if msg.Content == nil {
		return nil, nil
	}

	var content feishuTextContent
	if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
		return nil, err
	}

	text := strings.TrimSpace(content.Text)
	if text == "" {
		return nil, nil
	}

	openID := derefString(event.Event.Sender.SenderId.OpenId)
	userID := derefString(event.Event.Sender.SenderId.UserId)
	chatID := derefString(msg.ChatId)
	chatType := derefString(msg.ChatType)
	messageID := derefString(msg.MessageId)

	replyIDType := "open_id"
	replyID := openID
	if replyID == "" {
		replyIDType = "chat_id"
		replyID = chatID
	}

	return &feishuInboundMessage{
		text:        text,
		openID:      openID,
		userID:      userID,
		chatID:      chatID,
		chatType:    chatType,
		messageID:   messageID,
		replyIDType: replyIDType,
		replyID:     replyID,
	}, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isIncompleteFeishuAgentCommand(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || (!strings.HasPrefix(trimmed, "/agent") && !strings.HasPrefix(trimmed, "/to")) {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	command := fields[0]
	if idx := strings.IndexByte(command, '@'); idx != -1 {
		command = command[:idx]
	}
	if command != "/agent" && command != "/to" {
		return false
	}
	return len(fields) < 3
}

func resolveFeishuDomain(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case feishuDomainLark:
		return lark.LarkBaseUrl
	default:
		return lark.FeishuBaseUrl
	}
}

type FeishuAllowlist struct {
	configured bool
	allowed    map[string]struct{}
}

func NewFeishuAllowlist(values []string) FeishuAllowlist {
	allowed := make(map[string]struct{})
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	return FeishuAllowlist{configured: len(allowed) > 0, allowed: allowed}
}

func (a FeishuAllowlist) Allowed(openID, userID string) bool {
	if !a.configured {
		return false
	}
	if openID != "" {
		if _, ok := a.allowed[openID]; ok {
			return true
		}
	}
	if userID != "" {
		if _, ok := a.allowed[userID]; ok {
			return true
		}
	}
	return false
}
