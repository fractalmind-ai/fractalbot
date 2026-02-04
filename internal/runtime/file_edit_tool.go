package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const fileEditMaxBytes = 256 * 1024

// FileEditTool applies a deterministic in-place edit within sandbox roots.
type FileEditTool struct {
	sandbox PathSandbox
}

// NewFileEditTool creates a new file.edit tool.
func NewFileEditTool(sandbox PathSandbox) Tool {
	return FileEditTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileEditTool) Name() string {
	return "file.edit"
}

// Execute performs a single replacement edit.
func (t FileEditTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	path, payload, err := parseEditArgs(req.Args)
	if err != nil {
		return "", err
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
	if info.Size() > fileEditMaxBytes {
		return "", fmt.Errorf("file is too large")
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}

	updated, err := applyEditPayload(string(data), payload)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(safePath, []byte(updated)); err != nil {
		return "", fmt.Errorf("failed to write file")
	}
	return "ok", nil
}

type editPayload struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

func parseEditArgs(args string) (string, editPayload, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return "", editPayload{}, fmt.Errorf("path is required")
	}
	lines := strings.SplitN(trimmed, "\n", 2)
	path := strings.TrimSpace(lines[0])
	if path == "" {
		return "", editPayload{}, fmt.Errorf("path is required")
	}
	if len(lines) < 2 {
		return "", editPayload{}, fmt.Errorf("payload is required")
	}
	payloadRaw := strings.TrimLeft(lines[1], "\r\n")
	if strings.TrimSpace(payloadRaw) == "" {
		return "", editPayload{}, fmt.Errorf("payload is required")
	}
	var payload editPayload
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		return "", editPayload{}, fmt.Errorf("invalid payload")
	}
	if payload.OldText == "" {
		return "", editPayload{}, fmt.Errorf("oldText is required")
	}
	return path, payload, nil
}

func applyEditPayload(content string, payload editPayload) (string, error) {
	index := strings.Index(content, payload.OldText)
	if index == -1 {
		return "", fmt.Errorf("oldText not found")
	}
	var builder strings.Builder
	builder.Grow(len(content) - len(payload.OldText) + len(payload.NewText))
	builder.WriteString(content[:index])
	builder.WriteString(payload.NewText)
	builder.WriteString(content[index+len(payload.OldText):])
	return builder.String(), nil
}
