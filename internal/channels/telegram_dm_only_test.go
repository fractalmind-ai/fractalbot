package channels

import (
	"context"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type captureHandler struct {
	called bool
}

func (c *captureHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	c.called = true
	return "ok", nil
}

func TestTelegramGroupChatIgnored(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	handler := &captureHandler{}
	bot.SetHandler(handler)

	msg := &TelegramMessage{
		Text: "hello",
		From: &TelegramUser{ID: 123, UserName: "tester"},
		Chat: &TelegramChat{ID: 55, Type: "group"},
	}

	bot.handleIncomingMessage(msg)

	if handler.called {
		t.Fatalf("expected handler not called for group chat")
	}
	if payload.Text != "" {
		t.Fatalf("expected no reply for group chat, got %q", payload.Text)
	}
}
