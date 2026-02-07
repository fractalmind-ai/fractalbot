package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// FileExistsTool checks for file existence within sandbox roots.
type FileExistsTool struct {
	sandbox PathSandbox
}

// NewFileExistsTool creates a new file.exists tool.
func NewFileExistsTool(sandbox PathSandbox) Tool {
	return FileExistsTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileExistsTool) Name() string {
	return "file.exists"
}

// Execute checks for the existence of a sandboxed path.
func (t FileExistsTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	path := strings.TrimSpace(req.Args)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	safePath, err := t.sandbox.ValidatePath(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(safePath); err != nil {
		if os.IsNotExist(err) {
			return "false", nil
		}
		return "", fmt.Errorf("failed to access path")
	}
	return "true", nil
}
