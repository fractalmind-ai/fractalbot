package channels

import (
	"strings"
	"testing"
)

func TestTelegramAgentsIncludesDefault(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1", "coder-a"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	msg := &TelegramMessage{
		Text: "/agents",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 99},
	}

	handled, err := bot.handleCommand(msg)
	if !handled {
		t.Fatalf("expected handled")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(payload.Text, "Default agent: qa-1") {
		t.Fatalf("expected default agent line, got %q", payload.Text)
	}
	if strings.Count(payload.Text, "qa-1") != 1 {
		t.Fatalf("expected default agent listed once, got %q", payload.Text)
	}
}

func TestTelegramAgentsAllowlistUnset(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", nil)
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	msg := &TelegramMessage{
		Text: "/agents",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 99},
	}

	handled, err := bot.handleCommand(msg)
	if !handled {
		t.Fatalf("expected handled")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(payload.Text, "No agents configured") {
		t.Fatalf("unexpected empty agents reply: %q", payload.Text)
	}
	if !strings.Contains(payload.Text, "Default agent: qa-1") {
		t.Fatalf("expected default agent line, got %q", payload.Text)
	}
	if strings.Count(payload.Text, "qa-1") != 1 {
		t.Fatalf("expected default agent listed once, got %q", payload.Text)
	}
}

func TestTelegramAgentSelectionErrorIncludesHint(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	msg := &TelegramMessage{
		Text: "/agent hacker do stuff",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 99},
	}

	bot.handleIncomingMessage(msg)

	if !strings.Contains(payload.Text, "Tip: use /agents") {
		t.Fatalf("expected /agents hint, got %q", payload.Text)
	}
}
