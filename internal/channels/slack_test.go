package channels

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeSlackHandler struct {
	called bool
	reply  string
	err    error
	last   *protocol.Message
}

func (f *fakeSlackHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	f.called = true
	f.last = msg
	return f.reply, f.err
}

type fakeSlackLifecycle struct {
	monitorCalled bool
	monitorName   string
	monitorLines  int
	monitorReply  string
	monitorErr    error
}

func (f *fakeSlackLifecycle) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	_ = ctx
	_ = msg
	return "", nil
}

func (f *fakeSlackLifecycle) MonitorAgent(ctx context.Context, agentName string, lines int) (string, error) {
	_ = ctx
	f.monitorCalled = true
	f.monitorName = agentName
	f.monitorLines = lines
	return f.monitorReply, f.monitorErr
}

func (f *fakeSlackLifecycle) StartAgent(ctx context.Context, agentName string) (string, error) {
	_ = ctx
	_ = agentName
	return "", nil
}

func (f *fakeSlackLifecycle) StopAgent(ctx context.Context, agentName string) (string, error) {
	_ = ctx
	_ = agentName
	return "", nil
}

func (f *fakeSlackLifecycle) Doctor(ctx context.Context) (string, error) {
	_ = ctx
	return "", nil
}

type slackSendCapture struct {
	channelID string
	text      string
}

func TestSlackAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}
	if handler.last == nil {
		t.Fatalf("expected protocol message")
	}
	data, ok := handler.last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", handler.last.Data)
	}
	if data["channel"] != "slack" {
		t.Fatalf("expected channel slack, got %v", data["channel"])
	}
}

func TestSlackUnauthorized(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if handler.called {
		t.Fatalf("expected handler not called for unauthorized user")
	}
	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "U999") {
		t.Fatalf("expected user ID in reply, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "channels.slack.allowedUsers") {
		t.Fatalf("expected allowedUsers hint in reply, got %q", sent.text)
	}
}

func TestSlackSocketModeDMEvent(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleEventsAPIEvent(context.Background(), slackevents.EventsAPIEvent{
		Type: slackevents.CallbackEvent,
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Data: &slackevents.MessageEvent{
				Type:        "message",
				User:        "U123",
				Text:        "hello",
				Channel:     "D456",
				ChannelType: "im",
			},
		},
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}
}

func TestSlackWhoamiRequiresAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/whoami",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized for /whoami from non-allowed user, got %q", sent.text)
	}
}

func TestSlackAgentsEmptyConfigHint(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/agents",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected defaultAgent hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "agents.ohMyCode.allowedAgents") {
		t.Fatalf("expected allowedAgents hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agent <name> <task>") {
		t.Fatalf("expected /agent hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/to <name> <task>") {
		t.Fatalf("expected /to hint, got %q", sent.text)
	}
}

func TestSlackDefaultAgentMissingGuidance(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected config key hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agent <name> <task>") {
		t.Fatalf("expected /agent hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/to <name> <task>") {
		t.Fatalf("expected /to hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agents") {
		t.Fatalf("expected /agents hint, got %q", sent.text)
	}
}

func TestSlackAgentsDedupesDefault(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "qa-1", []string{"qa-1", "coder-a"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/agents",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Default agent: qa-1") {
		t.Fatalf("expected default agent line, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "coder-a") {
		t.Fatalf("expected allowlisted agent, got %q", sent.text)
	}
	if strings.Count(sent.text, "qa-1") != 1 {
		t.Fatalf("expected default agent listed once, got %q", sent.text)
	}
}

func TestSlackReplyTruncation(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeSlackHandler{reply: strings.Repeat("a", maxSlackReplyChars+10)}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "…(truncated)") {
		t.Fatalf("expected truncated reply, got %q", sent.text)
	}
}

func TestSlackIgnoreNonDMFromUnauthorized(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U999",
		channelID:   "C999",
		channelType: "channel",
	})

	if handler.called {
		t.Fatalf("expected handler not called for non-DM")
	}
	if sent.text != "" {
		t.Fatalf("expected no reply for non-DM, got %q", sent.text)
	}
}

func TestSlackIgnoreNonDMEvenFromAllowedUser(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "C999",
		channelType: "channel",
	})

	if handler.called {
		t.Fatalf("expected handler not called for non-DM even from allowed user")
	}
}

func TestSlackChannelAllowlistAuthorizes(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, []string{"C555"}, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	// Channel allowlist is tested via slashCommandReply (AppMentionEvent path)
	reply := bot.slashCommandReply(context.Background(), &slackInboundMessage{
		text:        "hello from channel",
		userID:      "U999",
		channelID:   "C555",
		channelType: "channel",
	})

	if !handler.called {
		t.Fatalf("expected handler called for user in allowed channel")
	}
	if reply != "ok" {
		t.Fatalf("expected reply ok, got %q", reply)
	}
	data := handler.last.Data.(map[string]interface{})
	if data["trust_level"] != "channel" {
		t.Fatalf("expected trust_level=channel, got %v", data["trust_level"])
	}
}

func TestSlackChannelAllowlistDeniesUnknownChannel(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, []string{"C555"}, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	// Channel allowlist is tested via slashCommandReply (AppMentionEvent path)
	reply := bot.slashCommandReply(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U999",
		channelID:   "C999",
		channelType: "channel",
	})

	if handler.called {
		t.Fatalf("expected handler not called for unknown channel")
	}
	if !strings.Contains(reply, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", reply)
	}
}

func TestSlackAllowedUserInChannelGetsTrustFull(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, []string{"C555"}, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	// allowedUser @mentions bot in a channel — trust_level should be "full"
	bot.slashCommandReply(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "C555",
		channelType: "channel",
	})

	if !handler.called {
		t.Fatalf("expected handler called for allowed user")
	}
	data := handler.last.Data.(map[string]interface{})
	if data["trust_level"] != "full" {
		t.Fatalf("expected trust_level=full for allowedUser, got %v", data["trust_level"])
	}
}

func TestSlackStatusDoesNotLeakTokens(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/status",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Bot Status") {
		t.Fatalf("expected status header, got %q", sent.text)
	}
	if strings.Contains(sent.text, "xoxb-secret") || strings.Contains(sent.text, "xapp-secret") {
		t.Fatalf("expected status to omit tokens, got %q", sent.text)
	}
}

func TestSlackStatusRequiresAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/status",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized for /status from non-allowed user, got %q", sent.text)
	}
}

func TestSlackAgentsRequiresAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/agents",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized for /agents from non-allowed user, got %q", sent.text)
	}
}

func TestSlackIncompleteAgentUsageRequiresAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	tests := []string{
		"/agent",
		"/agent qa-1",
		"/agent@bot",
		"/agent@bot qa-1",
		"/to",
		"/to qa-1",
		"/to@bot",
		"/to@bot qa-1",
	}
	for _, input := range tests {
		var sent slackSendCapture
		bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
			_ = ctx
			sent = slackSendCapture{channelID: channelID, text: text}
			return nil
		}

		bot.handleMessageEvent(context.Background(), &slackInboundMessage{
			text:        input,
			userID:      "U999",
			channelID:   "D456",
			channelType: "im",
		})

		if !strings.Contains(sent.text, "Unauthorized") {
			t.Fatalf("expected unauthorized for %q from non-allowed user, got %q", input, sent.text)
		}
	}
}

func TestSlackAgentWithTaskStillUnauthorized(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/agent qa-1 hello",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if handler.called {
		t.Fatalf("expected handler not called for unauthorized user")
	}
	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
}

func TestSlackHelpIncludesToAlias(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", nil, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	text := bot.helpText()
	if !strings.Contains(text, "/to <name> <task") {
		t.Fatalf("expected help text to include /to usage")
	}
	if !strings.Contains(text, "/agents") {
		t.Fatalf("expected help text to include /agents")
	}
	if !strings.Contains(text, "/tools") {
		t.Fatalf("expected help text to include /tools")
	}
	if !strings.Contains(text, "/tool <name>") {
		t.Fatalf("expected help text to include /tool")
	}
	if !strings.Contains(text, "allowlist") {
		t.Fatalf("expected help text to mention allowlist")
	}
	if !strings.Contains(text, "DM-only") {
		t.Fatalf("expected help text to mention DM-only")
	}
}

func TestSlackSlashCommandAckAllowlisted(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var ackText string
	bot.ackFn = func(req socketmode.Request, payload ...interface{}) {
		_ = req
		if len(payload) == 0 {
			return
		}
		if data, ok := payload[0].(map[string]string); ok {
			ackText = data["text"]
			return
		}
		if data, ok := payload[0].(map[string]interface{}); ok {
			if text, ok := data["text"].(string); ok {
				ackText = text
			}
		}
	}

	bot.handleSocketEvent(context.Background(), socketmode.Event{
		Type: socketmode.EventTypeSlashCommand,
		Data: slack.SlashCommand{
			Command:   "/help",
			Text:      "",
			UserID:    "U123",
			ChannelID: "C123",
		},
		Request: &socketmode.Request{EnvelopeID: "1"},
	})

	if !strings.Contains(ackText, "FractalBot Slack Help") {
		t.Fatalf("expected help in ack, got %q", ackText)
	}
}

func TestSlackSlashCommandAckUnauthorized(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var ackText string
	bot.ackFn = func(req socketmode.Request, payload ...interface{}) {
		_ = req
		if len(payload) == 0 {
			return
		}
		if data, ok := payload[0].(map[string]string); ok {
			ackText = data["text"]
			return
		}
		if data, ok := payload[0].(map[string]interface{}); ok {
			if text, ok := data["text"].(string); ok {
				ackText = text
			}
		}
	}

	bot.handleSocketEvent(context.Background(), socketmode.Event{
		Type: socketmode.EventTypeSlashCommand,
		Data: slack.SlashCommand{
			Command:   "/help",
			Text:      "",
			UserID:    "U999",
			ChannelID: "C123",
		},
		Request: &socketmode.Request{EnvelopeID: "1"},
	})

	if !strings.Contains(ackText, "Unauthorized") {
		t.Fatalf("expected unauthorized in ack, got %q", ackText)
	}
}

func TestSlackAppMentionAllowlisted(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleSocketEvent(context.Background(), socketmode.Event{
		Type: socketmode.EventTypeEventsAPI,
		Data: slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{Data: &slackevents.AppMentionEvent{
				User:    "U123",
				Channel: "C123",
				Text:    "<@B999> /help",
			}},
		},
		Request: &socketmode.Request{EnvelopeID: "1"},
	})

	if sent.channelID != "C123" {
		t.Fatalf("expected channel C123, got %q", sent.channelID)
	}
	if !strings.Contains(sent.text, "FractalBot Slack Help") {
		t.Fatalf("expected help reply, got %q", sent.text)
	}
}

func TestSlackAppMentionUnauthorized(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleSocketEvent(context.Background(), socketmode.Event{
		Type: socketmode.EventTypeEventsAPI,
		Data: slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{Data: &slackevents.AppMentionEvent{
				User:    "U999",
				Channel: "C123",
				Text:    "<@B999> /help",
			}},
		},
		Request: &socketmode.Request{EnvelopeID: "1"},
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
}

func TestSlackToolsRequiresAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "⚠️ runtime tools are disabled"}
	bot.SetHandler(handler)

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/tools",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized for /tools from non-allowed user, got %q", sent.text)
	}
}

func TestSlackToolBypassesAgentSelection(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/tool: echo hi",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}
}

func TestSlackToolHandlerMissing(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/tool: echo hi",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "runtime tools are disabled") {
		t.Fatalf("expected disabled reply, got %q", sent.text)
	}
}

func TestSlackMonitorUnauthorized(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackLifecycle{monitorReply: "logs"}
	bot.SetHandler(handler)

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/monitor qa-1 5",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if handler.monitorCalled {
		t.Fatalf("did not expect monitor to be called")
	}
	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
}

func TestSlackMonitorAllowed(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/monitor qa-1 5",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "agents.ohMyCode.enabled") {
		t.Fatalf("expected config hint, got %q", sent.text)
	}
}

func TestSlackMonitorUsage(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/monitor",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "usage: /monitor <name> [lines]") {
		t.Fatalf("expected usage reply, got %q", sent.text)
	}
}

func TestSlackToolCommandUnauthorized(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/tool: echo hi",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
}

func TestSlackToolCommandAllowedRoutes(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "hi"}
	bot.SetHandler(handler)

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/tool: echo hi",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if sent.text != "hi" {
		t.Fatalf("expected reply hi, got %q", sent.text)
	}
}

func TestSlackStatusWithMentionRequiresAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-secret", "xapp-secret", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/status@bot",
		userID:      "U999",
		channelID:   "D456",
		channelType: "im",
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized for /status@bot from non-allowed user, got %q", sent.text)
	}
}

func TestSlackAgentWithMentionRoutes(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "/agent@bot qa-1 hello",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if handler.last == nil {
		t.Fatalf("expected protocol message")
	}
	data, ok := handler.last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", handler.last.Data)
	}
	if data["text"] != "hello" {
		t.Fatalf("expected task text, got %v", data["text"])
	}
}

func TestNextSlackReconnectBackoff(t *testing.T) {
	if got := nextSlackReconnectBackoff(0); got != defaultSlackReconnectBackoffMin {
		t.Fatalf("backoff(0)=%s", got)
	}
	if got := nextSlackReconnectBackoff(defaultSlackReconnectBackoffMin); got != 2*defaultSlackReconnectBackoffMin {
		t.Fatalf("backoff(min)=%s", got)
	}
	if got := nextSlackReconnectBackoff(defaultSlackReconnectBackoffMax); got != defaultSlackReconnectBackoffMax {
		t.Fatalf("backoff(max)=%s", got)
	}
}

func TestSlackSocketModeReconnectsWithBackoff(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}
	bot.initClients()

	bot.socketClientFactoryFn = func(apiClient *slack.Client) *socketmode.Client {
		_ = apiClient
		return &socketmode.Client{}
	}
	bot.runSocketModeFn = func(ctx context.Context, socketClient *socketmode.Client) error {
		_ = ctx
		_ = socketClient
		return errors.New("socket closed")
	}

	var (
		mu     sync.Mutex
		waited []time.Duration
		done   = make(chan struct{})
	)
	bot.waitReconnectFn = func(ctx context.Context, backoff time.Duration) bool {
		_ = ctx
		mu.Lock()
		waited = append(waited, backoff)
		shouldStop := len(waited) >= 3
		mu.Unlock()
		if shouldStop {
			close(done)
			return false
		}
		return true
	}

	if err := bot.startSocketMode(context.Background()); err != nil {
		t.Fatalf("startSocketMode: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for reconnect attempts")
	}

	mu.Lock()
	got := append([]time.Duration(nil), waited...)
	mu.Unlock()

	want := []time.Duration{
		defaultSlackReconnectBackoffMin,
		2 * defaultSlackReconnectBackoffMin,
		4 * defaultSlackReconnectBackoffMin,
	}
	if len(got) != len(want) {
		t.Fatalf("waited=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("waited=%v want=%v", got, want)
		}
	}
}

func TestSlackMentionIncludesRecentMessages(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, []string{"C555"}, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		return nil
	}
	bot.fetchHistoryFn = func(ctx context.Context, channelID string, limit int) ([]map[string]interface{}, error) {
		_ = ctx
		return []map[string]interface{}{
			{"user": "U111", "text": "msg-1"},
			{"user": "U222", "text": "msg-2"},
		}, nil
	}

	// Trigger via AppMentionEvent path (slashCommandReply)
	bot.slashCommandReply(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "C555",
		channelType: "app_mention",
	})

	if !handler.called {
		t.Fatalf("expected handler called")
	}
	data := handler.last.Data.(map[string]interface{})
	recent, ok := data["recent_messages"].([]map[string]interface{})
	if !ok || len(recent) != 2 {
		t.Fatalf("expected 2 recent messages, got %v", data["recent_messages"])
	}
	if recent[0]["user"] != "U111" || recent[1]["text"] != "msg-2" {
		t.Fatalf("unexpected recent messages: %v", recent)
	}
}

func TestSlackDMIncludesRecentMessages(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		return nil
	}
	bot.fetchHistoryFn = func(ctx context.Context, channelID string, limit int) ([]map[string]interface{}, error) {
		_ = ctx
		return []map[string]interface{}{
			{"user": "U123", "text": "previous DM"},
		}, nil
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !handler.called {
		t.Fatalf("expected handler called")
	}
	data := handler.last.Data.(map[string]interface{})
	recent, ok := data["recent_messages"].([]map[string]interface{})
	if !ok || len(recent) != 1 {
		t.Fatalf("expected 1 recent message, got %v", data["recent_messages"])
	}
	if recent[0]["text"] != "previous DM" {
		t.Fatalf("unexpected recent message: %v", recent[0])
	}
}

func TestSlackHistoryFetchErrorDoesNotBlock(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	handler := &fakeSlackHandler{reply: "ok"}
	bot.SetHandler(handler)

	var sent slackSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = slackSendCapture{channelID: channelID, text: text}
		return nil
	}
	bot.fetchHistoryFn = func(ctx context.Context, channelID string, limit int) ([]map[string]interface{}, error) {
		_ = ctx
		return nil, errors.New("slack api error")
	}

	bot.handleMessageEvent(context.Background(), &slackInboundMessage{
		text:        "hello",
		userID:      "U123",
		channelID:   "D456",
		channelType: "im",
	})

	if !handler.called {
		t.Fatalf("expected handler called despite fetch error")
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}
	data := handler.last.Data.(map[string]interface{})
	if _, hasRecent := data["recent_messages"]; hasRecent {
		t.Fatalf("expected no recent_messages on fetch error, got %v", data["recent_messages"])
	}
}

func TestSlackMessageFromEventIncludesThreadTS(t *testing.T) {
	msg := slackMessageFromEvent(&slackevents.MessageEvent{
		User:            "U123",
		Channel:         "D456",
		ChannelType:     "im",
		Text:            "hello",
		ThreadTimeStamp: "1234567890.123456",
	})
	if msg == nil {
		t.Fatalf("expected parsed inbound message")
	}
	if msg.threadTS != "1234567890.123456" {
		t.Fatalf("threadTS=%q", msg.threadTS)
	}
}

func TestSlackMessageFromEventIncludesAttachments(t *testing.T) {
	msg := slackMessageFromEvent(&slackevents.MessageEvent{
		User:        "U123",
		Channel:     "D456",
		ChannelType: "im",
		Text:        "hello",
		Message: &slack.Msg{
			Files: []slack.File{
				{
					ID:         "F_IMAGE",
					Name:       "image.png",
					Mimetype:   "image/png",
					Filetype:   "png",
					URLPrivate: "https://files.slack.com/files-pri/T/F_IMAGE",
				},
				{
					ID:                 "F_DOC",
					Name:               "report.pdf",
					Mimetype:           "application/pdf",
					Filetype:           "pdf",
					URLPrivateDownload: "https://files.slack.com/files-pri/T/F_DOC/download",
				},
			},
		},
	})
	if msg == nil {
		t.Fatalf("expected parsed inbound message")
	}
	if len(msg.attachments) != 2 {
		t.Fatalf("attachments=%v", msg.attachments)
	}
	if msg.attachments[0].Type != "image" {
		t.Fatalf("attachment[0].type=%q", msg.attachments[0].Type)
	}
	if msg.attachments[1].Type != "file" {
		t.Fatalf("attachment[1].type=%q", msg.attachments[1].Type)
	}
	if msg.attachments[1].URL != "https://files.slack.com/files-pri/T/F_DOC/download" {
		t.Fatalf("attachment[1].url=%q", msg.attachments[1].URL)
	}
}

func TestSlackMessageFromAppMentionEventIncludesThreadTS(t *testing.T) {
	msg := slackMessageFromAppMentionEvent(&slackevents.AppMentionEvent{
		User:            "U123",
		Channel:         "C456",
		Text:            "<@B999> hello",
		ThreadTimeStamp: "1234567890.123456",
	})
	if msg == nil {
		t.Fatalf("expected parsed inbound mention message")
	}
	if msg.threadTS != "1234567890.123456" {
		t.Fatalf("threadTS=%q", msg.threadTS)
	}
}

func TestSlackReplyUsesThreadTSWhenPresent(t *testing.T) {
	var receivedThreadTS string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		receivedThreadTS = r.FormValue("thread_ts")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C0A8ESWV7D0","ts":"123.456","message":{"text":"thread reply"}}`))
	}))
	defer server.Close()

	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}
	bot.apiClient = slack.New("xoxb-token", slack.OptionAPIURL(server.URL+"/"))
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		t.Fatalf("expected reply to use thread API path, got plain sendMessage call")
		return nil
	}

	if err := bot.reply(context.Background(), &slackInboundMessage{
		channelID: "C0A8ESWV7D0",
		threadTS:  "1234567890.123456",
	}, "thread reply"); err != nil {
		t.Fatalf("reply: %v", err)
	}

	if receivedThreadTS != "1234567890.123456" {
		t.Fatalf("thread_ts=%q", receivedThreadTS)
	}
}

func TestSlackToProtocolMessageIncludesThreadTS(t *testing.T) {
	bot := &SlackBot{}
	msg := &slackInboundMessage{
		userID:      "U123",
		channelID:   "C456",
		channelType: "app_mention",
		threadTS:    "1234567890.123456",
	}

	protoMsg := bot.toProtocolMessage(msg, "hello", "qa-1", "full", nil)
	data, ok := protoMsg.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", protoMsg.Data)
	}
	if data["thread_ts"] != "1234567890.123456" {
		t.Fatalf("thread_ts=%v", data["thread_ts"])
	}
}

func TestSlackToProtocolMessageIncludesAttachments(t *testing.T) {
	bot := &SlackBot{}
	attachments := []protocol.Attachment{
		{
			Type:     "image",
			Filename: "image.png",
			URL:      "https://files.slack.com/files-pri/T/F_IMAGE",
			Channel:  "slack",
			MimeType: "image/png",
		},
	}
	msg := &slackInboundMessage{
		userID:      "U123",
		channelID:   "C456",
		channelType: "im",
		attachments: attachments,
	}

	protoMsg := bot.toProtocolMessage(msg, "hello", "qa-1", "full", nil)
	if len(protoMsg.Attachments) != 1 {
		t.Fatalf("protocol attachments=%v", protoMsg.Attachments)
	}
	data, ok := protoMsg.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", protoMsg.Data)
	}
	dataAttachments, ok := data["attachments"].([]protocol.Attachment)
	if !ok {
		t.Fatalf("expected typed data attachments, got %T", data["attachments"])
	}
	if len(dataAttachments) != 1 || dataAttachments[0].URL != attachments[0].URL {
		t.Fatalf("unexpected data attachments: %v", dataAttachments)
	}
}

func TestSlackSendMessageWithOptionsUsesThreadTS(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var captured slackSendCapture
	var capturedOpts SendOptions
	bot.sendMessageWithOptionsFn = func(ctx context.Context, channelID, text string, opts SendOptions) error {
		_ = ctx
		captured = slackSendCapture{channelID: channelID, text: text}
		capturedOpts = opts
		return nil
	}

	if err := bot.SendMessageWithOptions(
		context.Background(),
		"C0A8ESWV7D0",
		"thread reply",
		SendOptions{ThreadTS: "1234567890.123456"},
	); err != nil {
		t.Fatalf("SendMessageWithOptions: %v", err)
	}

	if captured.channelID != "C0A8ESWV7D0" {
		t.Fatalf("channelID=%q", captured.channelID)
	}
	if captured.text != "thread reply" {
		t.Fatalf("text=%q", captured.text)
	}
	if capturedOpts.ThreadTS != "1234567890.123456" {
		t.Fatalf("threadTS=%q", capturedOpts.ThreadTS)
	}
}

func TestSlackSendTextWithOptionsPostsThreadTS(t *testing.T) {
	var receivedChannel string
	var receivedText string
	var receivedThreadTS string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		receivedChannel = r.FormValue("channel")
		receivedText = r.FormValue("text")
		receivedThreadTS = r.FormValue("thread_ts")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C0A8ESWV7D0","ts":"123.456","message":{"text":"thread reply"}}`))
	}))
	defer server.Close()

	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}
	bot.apiClient = slack.New("xoxb-token", slack.OptionAPIURL(server.URL+"/"))

	if err := bot.sendTextWithOptions(
		context.Background(),
		"C0A8ESWV7D0",
		"thread reply",
		SendOptions{ThreadTS: "1234567890.123456"},
	); err != nil {
		t.Fatalf("sendTextWithOptions: %v", err)
	}

	if receivedChannel != "C0A8ESWV7D0" {
		t.Fatalf("channel=%q", receivedChannel)
	}
	if receivedText != "thread reply" {
		t.Fatalf("text=%q", receivedText)
	}
	if receivedThreadTS != "1234567890.123456" {
		t.Fatalf("thread_ts=%q", receivedThreadTS)
	}
}
