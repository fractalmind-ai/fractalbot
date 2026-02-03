package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileWriteTool writes files within sandbox roots using atomic writes.
type FileWriteTool struct {
	sandbox PathSandbox
}

// NewFileWriteTool creates a new file.write tool.
func NewFileWriteTool(sandbox PathSandbox) Tool {
	return FileWriteTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileWriteTool) Name() string {
	return "file.write"
}

// Execute writes content to a sandboxed path.
func (t FileWriteTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	path, content, err := parseWriteArgs(req.Args)
	if err != nil {
		return "", err
	}
	safePath, err := t.sandbox.ValidatePath(path)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(safePath, []byte(content)); err != nil {
		return "", fmt.Errorf("failed to write file")
	}
	return "ok", nil
}

func parseWriteArgs(args string) (string, string, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return "", "", fmt.Errorf("path is required")
	}
	lines := strings.SplitN(trimmed, "\n", 2)
	path := strings.TrimSpace(lines[0])
	if path == "" {
		return "", "", fmt.Errorf("path is required")
	}
	if len(lines) < 2 {
		return "", "", fmt.Errorf("content is required")
	}
	content := strings.TrimLeft(lines[1], "\r\n")
	if content == "" {
		return "", "", fmt.Errorf("content is required")
	}
	return path, content, nil
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(path)
		if err := os.Rename(tmpPath, path); err != nil {
			return err
		}
	}
	cleanup = false
	return nil
}
