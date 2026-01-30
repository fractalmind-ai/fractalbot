package channels

import (
	"strings"
	"testing"
)

func TestTelegramPing(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	msg := &TelegramMessage{
		Text: "/ping",
		From: &TelegramUser{ID: 123, UserName: "alice"},
		Chat: &TelegramChat{ID: 99},
	}

	handled, err := bot.handleCommand(msg)
	if !handled {
		t.Fatalf("expected handled")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(payload.Text) != "pong" {
		t.Fatalf("unexpected ping reply: %q", payload.Text)
	}
}
