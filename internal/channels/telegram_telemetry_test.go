package channels

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestTelegramTelemetry(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{1}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}
	bot.SetHandler(&fakeHandler{})

	update := telegramUpdate{
		Message: &TelegramMessage{
			From: &TelegramUser{ID: 1},
			Chat: &TelegramChat{ID: 1},
			Text: "hello",
		},
	}
	bot.handleUpdate(update)

	if bot.LastActivity().IsZero() {
		t.Fatalf("expected last activity to be set")
	}

	bot.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			_ = req
			return nil, errors.New("send failed")
		}),
	}
	if err := bot.SendMessage(context.Background(), 1, "hi"); err == nil {
		t.Fatalf("expected send error")
	}
	if bot.LastError().IsZero() {
		t.Fatalf("expected last error to be set")
	}
}
