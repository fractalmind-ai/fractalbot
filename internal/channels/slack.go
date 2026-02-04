package channels

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

// SlackBot implements a minimal Slack channel skeleton.
type SlackBot struct {
	botToken string
	appToken string

	allowlist    SlackAllowlist
	defaultAgent string
	agentAllow   AgentAllowlist

	handler IncomingMessageHandler

	apiClient    *slack.Client
	socketClient *socketmode.Client

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

func NewSlackBot(botToken, appToken string, allowedUsers []string, defaultAgent string, allowedAgents []string) (*SlackBot, error) {
	trimmedBot := strings.TrimSpace(botToken)
	trimmedApp := strings.TrimSpace(appToken)
	if trimmedBot == "" || trimmedApp == "" {
		return nil, errors.New("slack botToken and appToken are required")
	}

	return &SlackBot{
		botToken:     trimmedBot,
		appToken:     trimmedApp,
		allowlist:    NewSlackAllowlist(allowedUsers),
		defaultAgent: strings.TrimSpace(defaultAgent),
		agentAllow:   NewAgentAllowlist(allowedAgents),
		ctx:          context.Background(),
	}, nil
}

func (b *SlackBot) Name() string {
	return "slack"
}

func (b *SlackBot) SetHandler(handler IncomingMessageHandler) {
	b.handler = handler
}

func (b *SlackBot) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

// LastActivity reports the last time the bot saw a message or successfully sent one.
func (b *SlackBot) LastActivity() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastActivity
}

// LastError reports the last time the bot encountered a channel error.
func (b *SlackBot) LastError() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastError
}

func (b *SlackBot) markActivity() {
	b.telemetryMu.Lock()
	b.lastActivity = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *SlackBot) markError() {
	b.telemetryMu.Lock()
	b.lastError = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *SlackBot) setRunning(running bool) {
	b.runningMu.Lock()
	b.running = running
	b.runningMu.Unlock()
}

func (b *SlackBot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	if b.startFn == nil {
		b.initClients()
	}

	if b.startFn == nil {
		return errors.New("slack start function not configured")
	}

	if err := b.startFn(b.ctx); err != nil {
		return err
	}

	b.setRunning(true)
	return nil
}

func (b *SlackBot) Stop() error {
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

func (b *SlackBot) SendMessage(ctx context.Context, chatID int64, text string) error {
	_ = ctx
	_ = chatID
	_ = text
	return errors.New("slack SendMessage requires a string channel ID")
}

func (b *SlackBot) initClients() {
	if b.sendMessageFn != nil && b.startFn != nil {
		return
	}
	if b.botToken == "" || b.appToken == "" {
		return
	}
	b.apiClient = slack.New(b.botToken, slack.OptionAppLevelToken(b.appToken))
	b.socketClient = socketmode.New(b.apiClient)
	b.sendMessageFn = b.sendText
	b.startFn = b.startSocketMode
}

func (b *SlackBot) startSocketMode(ctx context.Context) error {
	if b.socketClient == nil {
		return errors.New("slack socket mode client not initialized")
	}

	go b.consumeSocketEvents(ctx)

	go func() {
		if err := b.socketClient.RunContext(ctx); err != nil {
			log.Printf("slack socket mode error: %v", err)
		}
	}()
	return nil
}

func (b *SlackBot) consumeSocketEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-b.socketClient.Events:
			if !ok {
				return
			}
			b.handleSocketEvent(ctx, event)
		}
	}
}

func (b *SlackBot) handleSocketEvent(ctx context.Context, event socketmode.Event) {
	if event.Request != nil {
		b.socketClient.Ack(*event.Request)
	}

	if event.Type != socketmode.EventTypeEventsAPI {
		return
	}
	eventsAPIEvent, ok := event.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}
	b.handleEventsAPIEvent(ctx, eventsAPIEvent)
}

func (b *SlackBot) handleEventsAPIEvent(ctx context.Context, event slackevents.EventsAPIEvent) {
	if event.Type != slackevents.CallbackEvent {
		return
	}
	switch ev := event.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		msg := slackMessageFromEvent(ev)
		if msg == nil {
			return
		}
		b.handleMessageEvent(ctx, msg)
	}
}

func (b *SlackBot) handleMessageEvent(ctx context.Context, msg *slackInboundMessage) {
	if msg == nil {
		return
	}
	if msg.channelType != "im" {
		return
	}

	b.markActivity()

	if isSlackSafeCommand(msg.text) {
		if handled, cmdErr := b.handleCommand(ctx, msg); handled {
			if cmdErr != nil {
				_ = b.reply(ctx, msg, fmt.Sprintf("❌ %v", cmdErr))
			}
			return
		}
	}

	if !b.allowlist.Allowed(msg.userID) {
		_ = b.reply(ctx, msg, "❌ Unauthorized. Ask an admin to add your Slack user ID to channels.slack.allowedUsers.\nTip: use /whoami to get your user ID.")
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
			log.Printf("slack handler error: %v", err)
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

func (b *SlackBot) handleCommand(ctx context.Context, msg *slackInboundMessage) (bool, error) {
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
	case "/status":
		return true, b.reply(ctx, msg, b.statusText())
	case "/agents":
		names := b.agentAllow.Names()
		defaultName := strings.TrimSpace(b.defaultAgent)
		if b.agentAllow.configured && defaultName != "" {
			names = filterOutAgentName(names, defaultName)
		}
		if len(names) == 0 {
			if defaultName == "" {
				return true, b.reply(ctx, msg, noAgentsConfiguredMessage)
			}
			if !b.agentAllow.configured {
				names = []string{defaultName}
			}
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
		reply := fmt.Sprintf("user_id: %s\nchannel_id: %s", msg.userID, msg.channelID)
		return true, b.reply(ctx, msg, reply)
	default:
		return true, fmt.Errorf("unknown command: %s", command)
	}
}

func (b *SlackBot) helpText() string {
	lines := []string{
		"FractalBot Slack Help",
		"",
		"Commands:",
		"  /help - show this help",
		"  /status - bot status",
		"  /agents - list allowed agents",
		"  /whoami - show your Slack IDs",
		"",
		"Agent routing:",
		"  /agent <name> <task...>",
	}
	return strings.Join(lines, "\n")
}

func (b *SlackBot) statusText() string {
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
		fmt.Sprintf("running: %t", b.IsRunning()),
		fmt.Sprintf("last_activity: %s", lastActivity),
		fmt.Sprintf("last_error: %s", lastError),
		fmt.Sprintf("default_agent: %s", defaultAgent),
		fmt.Sprintf("allowed_agents_configured: %t", allowlistConfigured),
	}, "\n")
}

func isSlackSafeCommand(text string) bool {
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

func (b *SlackBot) reply(ctx context.Context, msg *slackInboundMessage, text string) error {
	if b.sendMessageFn == nil {
		return errors.New("slack sender not configured")
	}
	if err := b.sendMessageFn(ctx, msg.channelID, TruncateSlackReply(text)); err != nil {
		b.markError()
		return err
	}
	b.markActivity()
	return nil
}

func (b *SlackBot) sendText(ctx context.Context, channelID, text string) error {
	if b.apiClient == nil {
		b.markError()
		return errors.New("slack api client not initialized")
	}
	if strings.TrimSpace(channelID) == "" {
		b.markError()
		return errors.New("slack channel ID is required")
	}
	_, _, err := b.apiClient.PostMessageContext(ctx, channelID, slack.MsgOptionText(text, false))
	if err != nil {
		b.markError()
		return err
	}
	return nil
}

func (b *SlackBot) toProtocolMessage(msg *slackInboundMessage, text, agent string) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel":    "slack",
			"text":       text,
			"agent":      agent,
			"user_id":    msg.userID,
			"channel_id": msg.channelID,
			"chatType":   msg.channelType,
		},
	}
}

type slackInboundMessage struct {
	text        string
	userID      string
	channelID   string
	channelType string
}

func slackMessageFromEvent(event *slackevents.MessageEvent) *slackInboundMessage {
	if event == nil {
		return nil
	}
	if event.SubType != "" {
		return nil
	}
	if strings.TrimSpace(event.User) == "" || strings.TrimSpace(event.Channel) == "" {
		return nil
	}
	return &slackInboundMessage{
		text:        event.Text,
		userID:      event.User,
		channelID:   event.Channel,
		channelType: event.ChannelType,
	}
}

type SlackAllowlist struct {
	configured bool
	allowed    map[string]struct{}
}

func NewSlackAllowlist(values []string) SlackAllowlist {
	allowed := make(map[string]struct{})
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	return SlackAllowlist{configured: len(allowed) > 0, allowed: allowed}
}

func (a SlackAllowlist) Allowed(userID string) bool {
	if !a.configured {
		return false
	}
	if userID == "" {
		return false
	}
	_, ok := a.allowed[userID]
	return ok
}
