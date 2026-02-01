package channels

import (
	"strings"
	"testing"
)

func TestTelegramDefaultAgentMissingGuidance(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	msg := &TelegramMessage{
		Text: "hello",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 99},
	}

	bot.handleIncomingMessage(msg)

	if !strings.Contains(payload.Text, "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected config key hint, got %q", payload.Text)
	}
	if !strings.Contains(payload.Text, "/agent <name> <task>") {
		t.Fatalf("expected /agent hint, got %q", payload.Text)
	}
	if !strings.Contains(payload.Text, "/agents") {
		t.Fatalf("expected /agents hint, got %q", payload.Text)
	}
}
