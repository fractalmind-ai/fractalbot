package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// FileSha256Tool returns a SHA-256 hash for a sandboxed file.
type FileSha256Tool struct {
	sandbox PathSandbox
}

// NewFileSha256Tool creates a new file.sha256 tool.
func NewFileSha256Tool(sandbox PathSandbox) Tool {
	return FileSha256Tool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileSha256Tool) Name() string {
	return "file.sha256"
}

// Execute returns the SHA-256 hash for a sandboxed file.
func (t FileSha256Tool) Execute(ctx context.Context, req ToolRequest) (string, error) {
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
	data, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
