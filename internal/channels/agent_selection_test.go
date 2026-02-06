package channels

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAgentSelection(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectTask  string
		expectAgent string
		specified   bool
		expectErr   bool
		errContains string
	}{
		{
			name:        "agent command",
			input:       "/agent coder-a do the thing",
			expectAgent: "coder-a",
			expectTask:  "do the thing",
			specified:   true,
		},
		{
			name:        "agent command with bot",
			input:       "/agent@bot coder_b run",
			expectAgent: "coder_b",
			expectTask:  "run",
			specified:   true,
		},
		{
			name:        "to command",
			input:       "/to coder-a do the thing",
			expectAgent: "coder-a",
			expectTask:  "do the thing",
			specified:   true,
		},
		{
			name:        "to command with bot",
			input:       "/to@bot coder_b run",
			expectAgent: "coder_b",
			expectTask:  "run",
			specified:   true,
		},
		{
			name:       "plain text",
			input:      "hello world",
			expectTask: "hello world",
			specified:  false,
		},
		{
			name:        "missing task",
			input:       "/agent coder",
			expectErr:   true,
			errContains: "usage: /agent <name> <task>",
		},
		{
			name:        "missing task to",
			input:       "/to coder",
			expectErr:   true,
			errContains: "usage: /to <name> <task>",
		},
		{
			name:        "missing task to with bot",
			input:       "/to@bot coder",
			expectErr:   true,
			errContains: "usage: /to <name> <task>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := ParseAgentSelection(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if selection.Task != tt.expectTask {
				t.Fatalf("task mismatch: got %q want %q", selection.Task, tt.expectTask)
			}
			if selection.Agent != tt.expectAgent {
				t.Fatalf("agent mismatch: got %q want %q", selection.Agent, tt.expectAgent)
			}
			if selection.Specified != tt.specified {
				t.Fatalf("specified mismatch: got %v want %v", selection.Specified, tt.specified)
			}
		})
	}
}

func TestResolveAgentSelection(t *testing.T) {
	allowlist := NewAgentAllowlist([]string{"coder-a", "coder_b"})

	selection := AgentSelection{Task: "do it"}
	resolved, err := ResolveAgentSelection(selection, "coder-a", allowlist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Agent != "coder-a" {
		t.Fatalf("expected default agent, got %q", resolved.Agent)
	}

	selection = AgentSelection{Agent: "coder_b", Task: "go", Specified: true}
	resolved, err = ResolveAgentSelection(selection, "coder-a", allowlist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Agent != "coder_b" {
		t.Fatalf("expected specified agent, got %q", resolved.Agent)
	}

	selection = AgentSelection{Agent: "coder-x", Task: "go", Specified: true}
	if _, err := ResolveAgentSelection(selection, "coder-a", allowlist); err == nil {
		t.Fatalf("expected allowlist error")
	}

	selection = AgentSelection{Task: "go"}
	if _, err := ResolveAgentSelection(selection, "", NewAgentAllowlist(nil)); err == nil {
		t.Fatalf("expected default agent error")
	} else if !errors.Is(err, errDefaultAgentMissing) {
		t.Fatalf("expected default agent missing error, got %v", err)
	}

	selection = AgentSelection{Agent: "-bad", Task: "go", Specified: true}
	if _, err := ResolveAgentSelection(selection, "coder-a", allowlist); err == nil {
		t.Fatalf("expected name validation error")
	}
}
