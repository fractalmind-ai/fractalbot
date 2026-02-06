package channels

import (
	"context"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type allowlistHandler struct {
	called bool
}

func (h *allowlistHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	h.called = true
	return "", nil
}

func TestTelegramAllowedChatsEmptyAllowsDM(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	handler := &allowlistHandler{}
	bot.SetHandler(handler)

	msg := &TelegramMessage{
		Text: "hello",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55, Type: "private"},
	}

	bot.handleIncomingMessage(msg)

	if !handler.called {
		t.Fatalf("expected handler called for allowed DM")
	}
}

func TestTelegramAllowedChatsRejectsUnknownChat(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}
	bot.setAllowedChats([]int64{999})

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	handler := &allowlistHandler{}
	bot.SetHandler(handler)

	msg := &TelegramMessage{
		Text: "hello",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55, Type: "private"},
	}

	bot.handleIncomingMessage(msg)

	if handler.called {
		t.Fatalf("expected handler not called for disallowed chat")
	}
	if payload.Text != "" {
		t.Fatalf("expected no reply for disallowed chat, got %q", payload.Text)
	}
}
