package agent

import (
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/internal/config"
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
