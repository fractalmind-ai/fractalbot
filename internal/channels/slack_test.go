package channels

import (
	"context"
	"encoding/json"
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
	if !strings.Contains(text, "/tool are intentionally unavailable") {
		t.Fatalf("expected help text to mark /tool as unavailable")
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

	handler := &fakeSlackHandler{reply: "⚠️ /tool and /tools are not available in gateway mode."}
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

	if !strings.Contains(sent.text, "not available in gateway mode") {
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
	if msg.attachments[1].Filename != "report.pdf" {
		t.Fatalf("attachment[1].filename=%q", msg.attachments[1].Filename)
	}
	if msg.attachments[1].MimeType != "application/pdf" {
		t.Fatalf("attachment[1].mimeType=%q", msg.attachments[1].MimeType)
	}
}

func TestSlackMessageFromEventIncludesLegacyAttachmentLinks(t *testing.T) {
	msg := slackMessageFromEvent(&slackevents.MessageEvent{
		User:        "U123",
		Channel:     "D456",
		ChannelType: "im",
		Text:        "please check this file",
		Message: &slack.Msg{
			Attachments: []slack.Attachment{
				{
					Title:     "product-spec.docx",
					TitleLink: "https://files.slack.com/files-pri/T/F_DOC/product-spec.docx?download=1",
				},
			},
		},
	})
	if msg == nil {
		t.Fatalf("expected parsed inbound message")
	}
	if len(msg.attachments) != 1 {
		t.Fatalf("attachments=%v", msg.attachments)
	}
	if msg.attachments[0].Filename != "product-spec.docx" {
		t.Fatalf("attachment.filename=%q", msg.attachments[0].Filename)
	}
	if msg.attachments[0].URL != "https://files.slack.com/files-pri/T/F_DOC/product-spec.docx?download=1" {
		t.Fatalf("attachment.url=%q", msg.attachments[0].URL)
	}
	if msg.attachments[0].MimeType != "" {
		t.Fatalf("attachment.mimeType=%q", msg.attachments[0].MimeType)
	}
}

func TestSlackHandleMessageEventForwardsAttachmentsToHandler(t *testing.T) {
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

	msg := slackMessageFromEvent(&slackevents.MessageEvent{
		User:        "U123",
		Channel:     "D456",
		ChannelType: "im",
		Text:        "please read attachment",
		Message: &slack.Msg{
			Files: []slack.File{
				{
					ID:         "F_DOC",
					Name:       "requirements.docx",
					Mimetype:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
					Filetype:   "docx",
					URLPrivate: "https://files.slack.com/files-pri/T/F_DOC",
				},
			},
		},
	})
	if msg == nil {
		t.Fatalf("expected parsed inbound message")
	}

	bot.handleMessageEvent(context.Background(), msg)

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}
	if len(handler.last.Attachments) != 1 {
		t.Fatalf("protocol attachments=%v", handler.last.Attachments)
	}
	if handler.last.Attachments[0].Filename != "requirements.docx" {
		t.Fatalf("protocol attachment filename=%q", handler.last.Attachments[0].Filename)
	}
	if handler.last.Attachments[0].URL != "https://files.slack.com/files-pri/T/F_DOC" {
		t.Fatalf("protocol attachment url=%q", handler.last.Attachments[0].URL)
	}
	if handler.last.Attachments[0].MimeType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("protocol attachment mimeType=%q", handler.last.Attachments[0].MimeType)
	}

	data, ok := handler.last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", handler.last.Data)
	}
	dataAttachments, ok := data["attachments"].([]protocol.Attachment)
	if !ok {
		t.Fatalf("expected typed data attachments, got %T", data["attachments"])
	}
	if len(dataAttachments) != 1 {
		t.Fatalf("data attachments=%v", dataAttachments)
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
	bot.sendMessageWithOptionsFn = bot.sendTextWithOptions
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

func TestSlackSendUsesThreadTS(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, nil, "", nil)
	if err != nil {
		t.Fatalf("NewSlackBot: %v", err)
	}

	var captured slackSendCapture
	var capturedThreadTS string
	bot.sendMessageWithOptionsFn = func(ctx context.Context, channelID, text, threadTS string) error {
		_ = ctx
		captured = slackSendCapture{channelID: channelID, text: text}
		capturedThreadTS = threadTS
		return nil
	}

	if err := bot.Send(
		context.Background(),
		OutboundMessage{
			To:       "C0A8ESWV7D0",
			Text:     "thread reply",
			ThreadTS: "1234567890.123456",
		},
	); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if captured.channelID != "C0A8ESWV7D0" {
		t.Fatalf("channelID=%q", captured.channelID)
	}
	if captured.text != "thread reply" {
		t.Fatalf("text=%q", captured.text)
	}
	if capturedThreadTS != "1234567890.123456" {
		t.Fatalf("threadTS=%q", capturedThreadTS)
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
		"1234567890.123456",
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

func TestSlackEventsParseMessageWithFilesViaJSON(t *testing.T) {
	// This test simulates the actual parsing path that socket mode uses.
	// It verifies that files at the top level of a message event are correctly
	// populated into event.Message.Files by the custom UnmarshalJSON.
	eventJSON := `{
		"token": "test-token",
		"team_id": "T123",
		"api_app_id": "A123",
		"type": "event_callback",
		"event": {
			"type": "message",
			"text": "check this doc",
			"user": "U123",
			"channel": "D456",
			"channel_type": "im",
			"ts": "1234567890.123456",
			"files": [
				{
					"id": "F123",
					"name": "report.docx",
					"mimetype": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
					"filetype": "docx",
					"url_private": "https://files.slack.com/files-pri/T123-F123/report.docx"
				}
			]
		}
	}`

	event, err := slackevents.ParseEvent(json.RawMessage(eventJSON), slackevents.OptionNoVerifyToken())
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}

	msgEvent, ok := event.InnerEvent.Data.(*slackevents.MessageEvent)
	if !ok {
		t.Fatalf("expected *MessageEvent, got %T", event.InnerEvent.Data)
	}

	// Check if custom unmarshaller populated Message
	if msgEvent.Message == nil {
		t.Fatalf("Message is nil — custom UnmarshalJSON did not populate it")
	}

	// Check if files were extracted from top-level JSON into Message.Files
	if len(msgEvent.Message.Files) == 0 {
		t.Fatalf("Message.Files is empty — files not parsed from event JSON")
	}

	if msgEvent.Message.Files[0].URLPrivate != "https://files.slack.com/files-pri/T123-F123/report.docx" {
		t.Fatalf("URLPrivate=%q", msgEvent.Message.Files[0].URLPrivate)
	}

	// Verify our slackAttachmentsFromEvent function works end-to-end
	attachments := slackAttachmentsFromEvent(msgEvent)
	if len(attachments) == 0 {
		t.Fatalf("slackAttachmentsFromEvent returned no attachments")
	}
	if attachments[0].URL != "https://files.slack.com/files-pri/T123-F123/report.docx" {
		t.Fatalf("attachment.URL=%q", attachments[0].URL)
	}
	if attachments[0].Filename != "report.docx" {
		t.Fatalf("attachment.Filename=%q", attachments[0].Filename)
	}
}

func TestSlackEventsParseMessageWithFilesAndEmptyMessageField(t *testing.T) {
	// This test reproduces a scenario where Slack sends a message event with
	// files at the top level AND an empty "message" field. The custom
	// UnmarshalJSON on MessageEvent sees Message != nil and skips populating
	// Files from the top level. fixupSlackMessageEventFiles corrects this.
	eventJSON := `{
		"token": "test-token",
		"team_id": "T123",
		"api_app_id": "A123",
		"type": "event_callback",
		"event": {
			"type": "message",
			"text": "check this doc",
			"user": "U123",
			"channel": "D456",
			"channel_type": "im",
			"ts": "1234567890.123456",
			"files": [
				{
					"id": "F123",
					"name": "report.docx",
					"mimetype": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
					"filetype": "docx",
					"url_private": "https://files.slack.com/files-pri/T123-F123/report.docx"
				}
			],
			"message": {}
		}
	}`

	event, err := slackevents.ParseEvent(json.RawMessage(eventJSON), slackevents.OptionNoVerifyToken())
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}

	msgEvent, ok := event.InnerEvent.Data.(*slackevents.MessageEvent)
	if !ok {
		t.Fatalf("expected *MessageEvent, got %T", event.InnerEvent.Data)
	}

	// Before fixup: Message.Files should be empty due to UnmarshalJSON behavior
	if len(msgEvent.Message.Files) != 0 {
		t.Fatalf("expected empty Files before fixup, got %d", len(msgEvent.Message.Files))
	}

	// Apply the fixup
	fixupSlackMessageEventFiles(msgEvent, event)

	// After fixup: Message.Files should be populated from raw JSON
	if len(msgEvent.Message.Files) == 0 {
		t.Fatalf("expected Files to be populated after fixup")
	}
	if msgEvent.Message.Files[0].URLPrivate != "https://files.slack.com/files-pri/T123-F123/report.docx" {
		t.Fatalf("URLPrivate=%q", msgEvent.Message.Files[0].URLPrivate)
	}

	// Verify slackAttachmentsFromEvent works after fixup
	attachments := slackAttachmentsFromEvent(msgEvent)
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Filename != "report.docx" {
		t.Fatalf("attachment.Filename=%q", attachments[0].Filename)
	}
	if attachments[0].URL != "https://files.slack.com/files-pri/T123-F123/report.docx" {
		t.Fatalf("attachment.URL=%q", attachments[0].URL)
	}
}

func TestSlackFixupDoesNotOverrideExistingFiles(t *testing.T) {
	// When Message.Files already has data, fixup should not replace it.
	eventJSON := `{
		"token": "test-token",
		"team_id": "T123",
		"api_app_id": "A123",
		"type": "event_callback",
		"event": {
			"type": "message",
			"text": "hello",
			"user": "U123",
			"channel": "D456",
			"channel_type": "im",
			"ts": "1234567890.123456",
			"files": [
				{
					"id": "F_TOP",
					"name": "top-level.docx",
					"url_private": "https://files.slack.com/top"
				}
			]
		}
	}`

	event, err := slackevents.ParseEvent(json.RawMessage(eventJSON), slackevents.OptionNoVerifyToken())
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}

	msgEvent := event.InnerEvent.Data.(*slackevents.MessageEvent)

	// For a message without "message" key, custom unmarshaller copies files correctly
	if len(msgEvent.Message.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(msgEvent.Message.Files))
	}

	// Fixup should be a no-op since files already exist
	fixupSlackMessageEventFiles(msgEvent, event)

	if len(msgEvent.Message.Files) != 1 {
		t.Fatalf("expected 1 file after fixup, got %d", len(msgEvent.Message.Files))
	}
	if msgEvent.Message.Files[0].ID != "F_TOP" {
		t.Fatalf("file ID=%q, expected F_TOP", msgEvent.Message.Files[0].ID)
	}
}

func TestResolveSlackMentions(t *testing.T) {
	bot := &SlackBot{
		resolveUsersFn: func(ctx context.Context) ([]slack.User, error) {
			return []slack.User{
				{ID: "U111", Name: "alice", Profile: slack.UserProfile{DisplayName: "Alice W"}, RealName: "Alice Wonderland"},
				{ID: "U222", Name: "bob", Profile: slack.UserProfile{DisplayName: "Bob"}, RealName: "Bob Builder"},
				{ID: "U333", Name: "charlie", Profile: slack.UserProfile{DisplayName: "Charlie"}, RealName: "Charlie Chaplin", Deleted: true},
				{ID: "U444", Name: "botuser", IsBot: true},
			}, nil
		},
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple mention",
			in:   "Hey @alice, check this",
			want: "Hey <@U111>, check this",
		},
		{
			name: "mention at start",
			in:   "@bob please review",
			want: "<@U222> please review",
		},
		{
			name: "case insensitive",
			in:   "cc @Alice",
			want: "cc <@U111>",
		},
		{
			name: "display name with space",
			in:   "Hey @bob, and @alice",
			want: "Hey <@U222>, and <@U111>",
		},
		{
			name: "already resolved mention unchanged",
			in:   "Hey <@U111> check this",
			want: "Hey <@U111> check this",
		},
		{
			name: "unknown user unchanged",
			in:   "Hey @unknown, check this",
			want: "Hey @unknown, check this",
		},
		{
			name: "deleted user not resolved",
			in:   "Hey @charlie",
			want: "Hey @charlie",
		},
		{
			name: "bot user not resolved",
			in:   "Hey @botuser",
			want: "Hey @botuser",
		},
		{
			name: "no mentions unchanged",
			in:   "Hello world",
			want: "Hello world",
		},
		{
			name: "email address not resolved",
			in:   "Send to user@example.com",
			want: "Send to user@example.com",
		},
		{
			name: "mention in parens",
			in:   "Assigned to (@alice)",
			want: "Assigned to (<@U111>)",
		},
		{
			name: "multiple mentions",
			in:   "@alice and @bob should review",
			want: "<@U111> and <@U222> should review",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bot.resolveSlackMentions(ctx, tt.in)
			if got != tt.want {
				t.Errorf("resolveSlackMentions(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveSlackMentionsCacheTTL(t *testing.T) {
	callCount := 0
	bot := &SlackBot{
		resolveUsersFn: func(ctx context.Context) ([]slack.User, error) {
			callCount++
			return []slack.User{
				{ID: "U111", Name: "alice"},
			}, nil
		},
	}

	ctx := context.Background()

	// First call populates the cache.
	got := bot.resolveSlackMentions(ctx, "@alice hi")
	if got != "<@U111> hi" {
		t.Fatalf("first call: got %q, want %q", got, "<@U111> hi")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", callCount)
	}

	// Second call should use cache, not call API again.
	got = bot.resolveSlackMentions(ctx, "@alice hi")
	if got != "<@U111> hi" {
		t.Fatalf("second call: got %q, want %q", got, "<@U111> hi")
	}
	if callCount != 1 {
		t.Fatalf("expected still 1 API call, got %d", callCount)
	}

	// Simulate stale cache by backdating the timestamp.
	bot.userDirMu.Lock()
	bot.userDirUpdated = time.Now().Add(-userDirTTL - time.Minute)
	bot.userDirMu.Unlock()

	got = bot.resolveSlackMentions(ctx, "@alice hi")
	if got != "<@U111> hi" {
		t.Fatalf("stale cache: got %q, want %q", got, "<@U111> hi")
	}
	if callCount != 2 {
		t.Fatalf("expected 2 API calls after TTL, got %d", callCount)
	}
}

func TestResolveSlackMentionsAPIError(t *testing.T) {
	bot := &SlackBot{
		resolveUsersFn: func(ctx context.Context) ([]slack.User, error) {
			return nil, errors.New("api error")
		},
	}

	ctx := context.Background()
	// Should return text unchanged when API fails.
	got := bot.resolveSlackMentions(ctx, "Hey @alice")
	if got != "Hey @alice" {
		t.Fatalf("got %q, want unchanged %q", got, "Hey @alice")
	}
}

func TestSendTextWithOptionsResolvesMentions(t *testing.T) {
	var sentText string
	bot := &SlackBot{
		apiClient: slack.New("xoxb-fake-token"),
		resolveUsersFn: func(ctx context.Context) ([]slack.User, error) {
			return []slack.User{
				{ID: "U111", Name: "alice"},
			}, nil
		},
	}

	// Override PostMessageContext by setting sendMessageWithOptionsFn to capture text.
	bot.sendMessageWithOptionsFn = func(ctx context.Context, channelID, text, threadTS string) error {
		sentText = text
		return nil
	}

	ctx := context.Background()
	err := bot.Send(ctx, OutboundMessage{To: "C123", Text: "Hey @alice, check this"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// sendMessageWithOptionsFn is called before sendTextWithOptions (which does resolution),
	// so we need to verify through the sendTextWithOptions path directly.
	sentText = ""
	// Use a test HTTP server to capture the sent text.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sentText = r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C123","ts":"1234.5678"}`))
	}))
	defer ts.Close()

	bot2 := &SlackBot{
		apiClient: slack.New("xoxb-fake-token", slack.OptionAPIURL(ts.URL+"/")),
		resolveUsersFn: func(ctx context.Context) ([]slack.User, error) {
			return []slack.User{
				{ID: "U111", Name: "alice"},
			}, nil
		},
	}

	err = bot2.sendTextWithOptions(ctx, "C123", "Hey @alice, check this", "")
	if err != nil {
		t.Fatalf("sendTextWithOptions: %v", err)
	}
	if sentText != "Hey <@U111>, check this" {
		t.Fatalf("sent text = %q, want %q", sentText, "Hey <@U111>, check this")
	}
}
