package channels

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestManagerRegistersIMessageChannel(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	manager := NewManager(&config.ChannelsConfig{
		IMessage: &config.IMessageConfig{
			Enabled:   true,
			Recipient: "recipient@example.com",
			Message:   "hello",
		},
	}, nil)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = manager.Stop() }()

	waitForCondition(t, time.Second, func() bool {
		channel := manager.Get("imessage")
		return channel != nil && channel.IsRunning()
	})
}

func TestManagerRejectsIMessageOnNonDarwin(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "linux"
	defer func() { currentGOOS = originalGOOS }()

	manager := NewManager(&config.ChannelsConfig{
		IMessage: &config.IMessageConfig{
			Enabled:   true,
			Recipient: "recipient@example.com",
		},
	}, nil)

	err := manager.Start(context.Background())
	if err == nil {
		t.Fatalf("expected non-darwin start error")
	}
	if !strings.Contains(err.Error(), "darwin") {
		t.Fatalf("expected darwin error, got %v", err)
	}
}
