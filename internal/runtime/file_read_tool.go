package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const fileReadMaxBytes = 256 * 1024

// FileReadTool reads files from within the sandbox roots.
type FileReadTool struct {
	sandbox PathSandbox
}

// NewFileReadTool creates a new file.read tool.
func NewFileReadTool(sandbox PathSandbox) Tool {
	return FileReadTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileReadTool) Name() string {
	return "file.read"
}

// Execute reads a file from a sandboxed path.
func (t FileReadTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
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
	if info.Size() > fileReadMaxBytes {
		return "", fmt.Errorf("file is too large")
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}
	return string(data), nil
}
