package channels

import (
	"context"
	"errors"
	"testing"
)

func TestFeishuTelemetry(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_allowed"}, "", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		_ = ctx
		_ = receiveIDType
		_ = receiveID
		_ = text
		return errors.New("send failed")
	}

	event := buildFeishuEvent("hello", "p2p", "ou_allowed", "u1", "chat1")
	if err := bot.handleMessageEvent(context.Background(), event); err != nil {
		t.Fatalf("handleMessageEvent: %v", err)
	}

	if bot.LastActivity().IsZero() {
		t.Fatalf("expected last activity to be set")
	}
	if bot.LastError().IsZero() {
		t.Fatalf("expected last error to be set")
	}
}
