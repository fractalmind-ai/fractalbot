package channels

import (
	"context"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeIncomingHandler struct {
	called bool
}

func (f *fakeIncomingHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	f.called = true
	return "", nil
}

func TestTelegramUnauthorizedHint(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	handler := &fakeIncomingHandler{}
	bot.SetHandler(handler)

	msg := &TelegramMessage{
		Text: "/ping",
		From: &TelegramUser{ID: 999, UserName: "intruder"},
		Chat: &TelegramChat{ID: 55},
	}

	bot.handleIncomingMessage(msg)

	if handler.called {
		t.Fatalf("expected handler not called for unauthorized user")
	}
	if !strings.Contains(payload.Text, "/whoami") {
		t.Fatalf("expected /whoami hint in reply: %q", payload.Text)
	}
	if !strings.Contains(payload.Text, "allowedUsers") {
		t.Fatalf("expected allowedUsers hint in reply: %q", payload.Text)
	}
}
