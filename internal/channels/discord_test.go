package channels

import (
	"context"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeDiscordHandler struct {
	called bool
	reply  string
	err    error
	last   *protocol.Message
}

func (f *fakeDiscordHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	f.called = true
	f.last = msg
	return f.reply, f.err
}

type discordSendCapture struct {
	channelID string
	text      string
}

func TestDiscordAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "hello",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
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
	if data["channel"] != "discord" {
		t.Fatalf("expected channel discord, got %v", data["channel"])
	}
}

func TestDiscordUnauthorized(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "hello",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if handler.called {
		t.Fatalf("expected handler not called for unauthorized user")
	}
	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
}

func TestDiscordWhoamiAllowedWithoutAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/whoami",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "user_id: 999") {
		t.Fatalf("expected whoami reply, got %q", sent.text)
	}
	if strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("did not expect unauthorized for /whoami, got %q", sent.text)
	}
}

func TestDiscordIgnoreNonDM(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "hello",
		userID:      "123",
		channelID:   "C123",
		channelType: "channel",
	})

	if handler.called {
		t.Fatalf("expected handler not called for non-DM")
	}
	if sent.text != "" {
		t.Fatalf("expected no reply for non-DM, got %q", sent.text)
	}
}
