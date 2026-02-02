package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestRuntimeUnknownToolDenied(t *testing.T) {
	rt, err := NewRuntime(&config.RuntimeConfig{
		Enabled:      true,
		AllowedTools: []string{"echo"},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	reply, err := rt.HandleTask(context.Background(), Task{
		Agent:   "qa-1",
		Text:    "tool unknown hi",
		Channel: "telegram",
	})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if !strings.Contains(reply, "unknown tool") {
		t.Fatalf("expected unknown tool error, got %q", reply)
	}
}

func TestRuntimeAllowlistedToolAllowed(t *testing.T) {
	rt, err := NewRuntime(&config.RuntimeConfig{
		Enabled:      true,
		AllowedTools: []string{"echo"},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	reply, err := rt.HandleTask(context.Background(), Task{
		Agent:   "qa-1",
		Text:    "tool echo hello",
		Channel: "telegram",
	})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if reply != "hello" {
		t.Fatalf("expected echo reply, got %q", reply)
	}
}

func TestRuntimeToolOutputTruncation(t *testing.T) {
	rt, err := NewRuntime(&config.RuntimeConfig{
		Enabled:       true,
		AllowedTools:  []string{"echo"},
		MaxReplyChars: 12,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	reply, err := rt.HandleTask(context.Background(), Task{
		Agent:   "qa-1",
		Text:    "tool echo " + strings.Repeat("a", 50),
		Channel: "telegram",
	})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if !strings.Contains(reply, "...(truncated)") {
		t.Fatalf("expected truncated suffix, got %q", reply)
	}
}
