package channels

import (
	"context"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeReplyHandler struct {
	reply string
	err   error
}

func (f *fakeReplyHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	return f.reply, f.err
}

func TestTelegramHandlerReplyTruncated(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	longText := strings.Repeat("x", 4000)
	bot.SetHandler(&fakeReplyHandler{reply: longText})

	msg := &TelegramMessage{
		Text: "hello",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55},
	}

	bot.handleIncomingMessage(msg)

	if !strings.Contains(payload.Text, "…(truncated)") {
		t.Fatalf("expected truncated suffix, got %q", payload.Text)
	}
}

func TestTelegramHandlerErrorSanitized(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	sentinel := "SECRET_ERROR"
	bot.SetHandler(&fakeReplyHandler{err: errSentinel(sentinel)})

	msg := &TelegramMessage{
		Text: "hello",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55},
	}

	bot.handleIncomingMessage(msg)

	if strings.Contains(payload.Text, sentinel) {
		t.Fatalf("expected sanitized error, got %q", payload.Text)
	}
	if !strings.Contains(payload.Text, "Something went wrong") {
		t.Fatalf("expected generic error, got %q", payload.Text)
	}
}

func TestTelegramToolsCommandRoutedToHandler(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	const sentinel = "tools routed"
	bot.SetHandler(&fakeReplyHandler{reply: sentinel})

	msg := &TelegramMessage{
		Text: "/tools",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55},
	}

	bot.handleIncomingMessage(msg)

	if payload.Text != sentinel {
		t.Fatalf("expected handler reply %q, got %q", sentinel, payload.Text)
	}
}

func TestTelegramToolCommandRoutedToHandler(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	var payload sendMessagePayload
	bot.httpClient = captureHTTPClient(t, &payload)

	const sentinel = "tool routed"
	bot.SetHandler(&fakeReplyHandler{reply: sentinel})

	msg := &TelegramMessage{
		Text: "/tool: echo hi",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55},
	}

	bot.handleIncomingMessage(msg)

	if payload.Text != sentinel {
		t.Fatalf("expected handler reply %q, got %q", sentinel, payload.Text)
	}
}

type errSentinel string

func (e errSentinel) Error() string {
	return string(e)
}
