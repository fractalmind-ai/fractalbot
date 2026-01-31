package channels

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func okTelegramBoolResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`)),
		Header:     make(http.Header),
	}
}

func TestTelegramWebhookRegisterOnStart(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	bot.ConfigureMode("webhook")
	bot.ConfigureWebhook("", "/telegram/webhook", "https://example.com/telegram/webhook", "")
	bot.ConfigureWebhookLifecycle(true, false)

	setCalls := 0
	bot.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "setWebhook") {
				setCalls++
			}
			return okTelegramBoolResponse(), nil
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := bot.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if setCalls != 1 {
		t.Fatalf("expected setWebhook call, got %d", setCalls)
	}
	if err := bot.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestTelegramWebhookDeleteOnStop(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	bot.ConfigureMode("webhook")
	bot.ConfigureWebhook("", "/telegram/webhook", "https://example.com/telegram/webhook", "")
	bot.ConfigureWebhookLifecycle(false, true)

	deleteCalls := 0
	bot.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "deleteWebhook") {
				deleteCalls++
			}
			return okTelegramBoolResponse(), nil
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := bot.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if deleteCalls != 0 {
		t.Fatalf("expected deleteWebhook not called on start")
	}
	if err := bot.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("expected deleteWebhook call, got %d", deleteCalls)
	}
}
