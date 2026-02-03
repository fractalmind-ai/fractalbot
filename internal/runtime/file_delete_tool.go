package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// FileDeleteTool deletes files within sandbox roots.
type FileDeleteTool struct {
	sandbox PathSandbox
}

// NewFileDeleteTool creates a new file.delete tool.
func NewFileDeleteTool(sandbox PathSandbox) Tool {
	return FileDeleteTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileDeleteTool) Name() string {
	return "file.delete"
}

// Execute deletes a sandboxed file.
func (t FileDeleteTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	path := strings.TrimSpace(req.Args)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	safePath, err := t.sandbox.ValidatePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to access file")
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	if err := os.Remove(safePath); err != nil {
		return "", fmt.Errorf("failed to delete file")
	}
	return "ok", nil
}
