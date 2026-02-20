package agent

import (
	"context"
	"os"
	"path/filepath"
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

func TestAssignOhMyCodeReturnsAckWithoutMonitor(t *testing.T) {
	workspace := t.TempDir()
	scriptPath := filepath.Join(workspace, "agent_manager_stub.py")
	logPath := filepath.Join(workspace, "calls.log")

	script := `import pathlib
import sys

log_path = pathlib.Path(sys.argv[0]).with_name("calls.log")
with log_path.open("a", encoding="utf-8") as f:
    f.write(" ".join(sys.argv[1:]) + "\n")

if len(sys.argv) >= 2 and sys.argv[1] == "assign":
    print("assign ok")
    sys.exit(0)

if len(sys.argv) >= 2 and sys.argv[1] == "monitor":
    print("monitor should not run")
    sys.exit(0)

print("unknown command", file=sys.stderr)
sys.exit(1)
`

	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	manager := NewManager(&config.AgentsConfig{
		OhMyCode: &config.OhMyCodeConfig{
			Enabled:            true,
			Workspace:          workspace,
			AgentManagerScript: scriptPath,
			DefaultAgent:       "qa-1",
		},
	})

	out, err := manager.assignOhMyCode(context.Background(), "hello world", "")
	if err != nil {
		t.Fatalf("assignOhMyCode: %v", err)
	}
	if out != ohMyCodeAssignAckMessage {
		t.Fatalf("expected %q, got %q", ohMyCodeAssignAckMessage, out)
	}

	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read calls log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(logRaw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only one agent-manager call, got %d (%q)", len(lines), lines)
	}
	if lines[0] != "assign qa-1" {
		t.Fatalf("expected assign call, got %q", lines[0])
	}
}

func TestAssignOhMyCodePreservesAssignFailure(t *testing.T) {
	workspace := t.TempDir()
	scriptPath := filepath.Join(workspace, "agent_manager_fail.py")

	script := `import sys

if len(sys.argv) >= 2 and sys.argv[1] == "assign":
    print("assign failed", file=sys.stderr)
    sys.exit(2)

print("unexpected command", file=sys.stderr)
sys.exit(1)
`

	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	manager := NewManager(&config.AgentsConfig{
		OhMyCode: &config.OhMyCodeConfig{
			Enabled:            true,
			Workspace:          workspace,
			AgentManagerScript: scriptPath,
			DefaultAgent:       "qa-1",
		},
	})

	out, err := manager.assignOhMyCode(context.Background(), "hello world", "")
	if err == nil {
		t.Fatal("expected assign failure")
	}
	if out != "" {
		t.Fatalf("expected empty output on failure, got %q", out)
	}
	if !strings.Contains(err.Error(), "assign failed") {
		t.Fatalf("expected assign failure error, got %v", err)
	}
}
