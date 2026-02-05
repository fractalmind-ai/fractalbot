package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

func TestValidateOhMyCodeAgentDefaultOnly(t *testing.T) {
	manager := NewManager(&config.AgentsConfig{
		OhMyCode: &config.OhMyCodeConfig{
			Enabled:      true,
			Workspace:    "/tmp",
			DefaultAgent: "qa-1",
		},
	})

	if _, err := manager.validateOhMyCodeAgent("qa-1"); err != nil {
		t.Fatalf("expected default agent allowed: %v", err)
	}
	if _, err := manager.validateOhMyCodeAgent("coder-a"); err == nil {
		t.Fatal("expected non-default agent to be rejected")
	}
}

func TestValidateOhMyCodeAgentAllowlist(t *testing.T) {
	manager := NewManager(&config.AgentsConfig{
		OhMyCode: &config.OhMyCodeConfig{
			Enabled:       true,
			Workspace:     "/tmp",
			DefaultAgent:  "qa-1",
			AllowedAgents: []string{"qa-1", "coder-a"},
		},
	})

	if _, err := manager.validateOhMyCodeAgent("coder-a"); err != nil {
		t.Fatalf("expected allowlisted agent allowed: %v", err)
	}
	if _, err := manager.validateOhMyCodeAgent("coder-b"); err == nil {
		t.Fatal("expected non-allowlisted agent rejected")
	}
}

func TestValidateOhMyCodeAgentInvalidName(t *testing.T) {
	manager := NewManager(&config.AgentsConfig{
		OhMyCode: &config.OhMyCodeConfig{
			Enabled:      true,
			Workspace:    "/tmp",
			DefaultAgent: "qa-1",
		},
	})

	if _, err := manager.validateOhMyCodeAgent(""); err == nil {
		t.Fatal("expected empty name rejected")
	}
	if _, err := manager.validateOhMyCodeAgent("bad name"); err == nil {
		t.Fatal("expected invalid name rejected")
	} else if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid name error, got %v", err)
	}
}

func TestBuildOhMyCodeTaskPrompt(t *testing.T) {
	out := buildOhMyCodeTaskPrompt("hello world")
	if !strings.HasPrefix(out, "User message:\n") {
		t.Fatalf("expected user message prefix, got %q", out)
	}
	if strings.Contains(out, "Telegram") {
		t.Fatalf("did not expect channel-specific wording, got %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("expected message content, got %q", out)
	}
}

func TestHandleIncomingToolDisabledWhenRuntimeOff(t *testing.T) {
	manager := NewManager(&config.AgentsConfig{
		OhMyCode: &config.OhMyCodeConfig{
			Enabled:      true,
			Workspace:    "/tmp",
			DefaultAgent: "qa-1",
		},
	})

	tests := []string{"/tools", "/tool: echo hi", "tool echo hi"}
	for _, input := range tests {
		msg := &protocol.Message{
			Data: map[string]interface{}{
				"channel": "slack",
				"text":    input,
			},
		}
		out, err := manager.HandleIncoming(context.Background(), msg)
		if err != nil {
			t.Fatalf("HandleIncoming: %v", err)
		}
		if !strings.Contains(out, "runtime tools are disabled") {
			t.Fatalf("expected disabled message for %q, got %q", input, out)
		}
		if !strings.Contains(out, "agents.runtime.enabled") {
			t.Fatalf("expected enabled config hint for %q, got %q", input, out)
		}
		if !strings.Contains(out, "agents.runtime.allowedTools") {
			t.Fatalf("expected allowedTools config hint for %q, got %q", input, out)
		}
	}
}
