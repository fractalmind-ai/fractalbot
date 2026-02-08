package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const execHelperEnv = "FRACTALBOT_CMD_EXEC_HELPER"
const execHelperSleepEnv = "FRACTALBOT_CMD_EXEC_HELPER_SLEEP"

func TestCommandExecHelper(t *testing.T) {
	if os.Getenv(execHelperEnv) != "1" {
		return
	}
	if os.Getenv(execHelperSleepEnv) == "1" {
		time.Sleep(2 * time.Second)
		return
	}
	_, _ = os.Stdout.WriteString("helper-ok")
	os.Exit(0)
}

func TestCommandExecToolRejectsEmptyRoots(t *testing.T) {
	tool := NewCommandExecTool(PathSandbox{}, []string{"echo"})
	args := mustJSONArgs(commandExecArgs{Command: []string{"echo", "hi"}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: args}); err == nil {
		t.Fatal("expected error for empty roots")
	} else if !strings.Contains(err.Error(), "agents.runtime.sandboxRoots") {
		t.Fatalf("expected sandboxRoots hint, got %q", err.Error())
	}
}

func TestCommandExecToolRejectsEmptyAllowlist(t *testing.T) {
	root := t.TempDir()
	tool := NewCommandExecTool(PathSandbox{Roots: []string{root}}, nil)
	args := mustJSONArgs(commandExecArgs{Command: []string{os.Args[0], "-test.run=TestCommandExecHelper"}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: args}); err == nil {
		t.Fatal("expected error for empty allowlist")
	} else if !strings.Contains(err.Error(), "agents.runtime.commandExec.allowlist") {
		t.Fatalf("expected allowlist hint, got %q", err.Error())
	}
}

func TestCommandExecToolRejectsNotAllowedCommand(t *testing.T) {
	root := t.TempDir()
	tool := NewCommandExecTool(PathSandbox{Roots: []string{root}}, []string{"not-allowed"})
	args := mustJSONArgs(commandExecArgs{Command: []string{os.Args[0], "-test.run=TestCommandExecHelper"}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: args}); err == nil {
		t.Fatal("expected error for not-allowed command")
	} else if !strings.Contains(err.Error(), "agents.runtime.commandExec.allowlist") {
		t.Fatalf("expected allowlist hint, got %q", err.Error())
	}
}

func TestCommandExecToolRejectsOutsideRootCwd(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	tool := NewCommandExecTool(PathSandbox{Roots: []string{root}}, []string{"echo"})
	args := mustJSONArgs(commandExecArgs{
		Command: []string{"echo", "hi"},
		Cwd:     outside,
	})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: args}); err == nil {
		t.Fatal("expected error for outside root")
	}
}

func TestCommandExecToolRunsCommand(t *testing.T) {
	root := t.TempDir()
	tool := NewCommandExecTool(PathSandbox{Roots: []string{root}}, []string{filepath.Base(os.Args[0])})
	args := mustJSONArgs(commandExecArgs{
		Command: []string{os.Args[0], "-test.run=TestCommandExecHelper"},
		Cwd:     root,
	})
	if err := os.Setenv(execHelperEnv, "1"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	defer os.Unsetenv(execHelperEnv)

	output, err := tool.Execute(context.Background(), ToolRequest{Args: args})
	if err != nil {
		t.Fatalf("expected command to succeed: %v", err)
	}
	if output != "helper-ok" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestCommandExecToolTimeout(t *testing.T) {
	root := t.TempDir()
	tool := NewCommandExecTool(PathSandbox{Roots: []string{root}}, []string{filepath.Base(os.Args[0])})
	args := mustJSONArgs(commandExecArgs{
		Command:   []string{os.Args[0], "-test.run=TestCommandExecHelper"},
		Cwd:       root,
		TimeoutMs: 10,
	})
	if err := os.Setenv(execHelperEnv, "1"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	if err := os.Setenv(execHelperSleepEnv, "1"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	defer os.Unsetenv(execHelperEnv)
	defer os.Unsetenv(execHelperSleepEnv)

	if _, err := tool.Execute(context.Background(), ToolRequest{Args: args}); err == nil {
		t.Fatal("expected timeout error")
	}
}

func mustJSONArgs(args commandExecArgs) string {
	data, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return string(data)
}
