package channels

import (
	"context"
	"strings"
	"testing"

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

type slackSendCapture struct {
	channelID string
	text      string
}

func TestSlackAllowlist(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, "", nil)
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
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, "", nil)
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
}

func TestSlackIgnoreNonDM(t *testing.T) {
	bot, err := NewSlackBot("xoxb-token", "xapp-token", []string{"U123"}, "", nil)
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
