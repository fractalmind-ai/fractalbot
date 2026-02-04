package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultCommandTimeout = 10 * time.Second
	maxCommandTimeout     = 60 * time.Second
)

// CommandExecTool executes commands in a sandboxed working directory.
type CommandExecTool struct {
	sandbox   PathSandbox
	allowlist map[string]struct{}
}

// NewCommandExecTool creates a new command.exec tool.
func NewCommandExecTool(sandbox PathSandbox, allowlist []string) Tool {
	return CommandExecTool{
		sandbox:   sandbox,
		allowlist: normalizeCommandAllowlist(allowlist),
	}
}

// Name returns the tool name.
func (CommandExecTool) Name() string {
	return "command.exec"
}

// Execute runs a command with argv arguments.
func (t CommandExecTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	parsed, err := parseCommandExecArgs(req.Args)
	if err != nil {
		return "", err
	}
	if len(parsed.Command) == 0 || strings.TrimSpace(parsed.Command[0]) == "" {
		return "", fmt.Errorf("command is required")
	}
	if len(t.allowlist) == 0 {
		return "", fmt.Errorf("command allowlist is not configured")
	}
	if !t.isAllowed(parsed.Command[0]) {
		return "", fmt.Errorf("command is not allowed")
	}
	if len(t.sandbox.Roots) == 0 {
		return "", fmt.Errorf("sandbox roots are not configured")
	}

	cwd := strings.TrimSpace(parsed.Cwd)
	if cwd == "" {
		cwd = t.sandbox.Roots[0]
	}
	safeCwd, err := t.sandbox.ValidatePath(cwd)
	if err != nil {
		return "", err
	}

	timeout := defaultCommandTimeout
	if parsed.TimeoutMs > 0 {
		timeout = time.Duration(parsed.TimeoutMs) * time.Millisecond
	}
	if timeout > maxCommandTimeout {
		timeout = maxCommandTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, parsed.Command[0], parsed.Command[1:]...)
	cmd.Dir = safeCwd

	stdout := newLimitedBuffer(defaultMaxReplyChars)
	stderr := newLimitedBuffer(defaultMaxReplyChars)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("command timed out")
		}
		return "", fmt.Errorf("command failed")
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}
	if output == "" {
		return "ok", nil
	}
	if stdout.Truncated() || stderr.Truncated() {
		output = strings.TrimSpace(output) + truncateSuffix
	}
	return output, nil
}

func normalizeCommandAllowlist(allowlist []string) map[string]struct{} {
	normalized := make(map[string]struct{})
	for _, entry := range allowlist {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		base := filepath.Base(trimmed)
		if base == "." || base == string(filepath.Separator) || base == "" {
			continue
		}
		normalized[base] = struct{}{}
	}
	return normalized
}

func (t CommandExecTool) isAllowed(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return false
	}
	base := filepath.Base(trimmed)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return false
	}
	_, ok := t.allowlist[base]
	return ok
}

type commandExecArgs struct {
	Command   []string `json:"command"`
	Cwd       string   `json:"cwd"`
	TimeoutMs int      `json:"timeoutMs"`
}

func parseCommandExecArgs(args string) (commandExecArgs, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return commandExecArgs{}, fmt.Errorf("args are required")
	}
	var parsed commandExecArgs
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return commandExecArgs{}, fmt.Errorf("invalid args")
	}
	return parsed, nil
}

type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func newLimitedBuffer(max int) *limitedBuffer {
	if max < 0 {
		max = 0
	}
	return &limitedBuffer{max: max}
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.max == 0 {
		l.truncated = true
		return len(p), nil
	}
	remaining := l.max - l.buf.Len()
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		l.buf.Write(p[:remaining])
		l.truncated = true
		return len(p), nil
	}
	l.buf.Write(p)
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

func (l *limitedBuffer) Truncated() bool {
	return l.truncated
}
