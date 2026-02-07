package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FileStatTool returns basic metadata for a sandboxed path.
type FileStatTool struct {
	sandbox PathSandbox
}

// NewFileStatTool creates a new file.stat tool.
func NewFileStatTool(sandbox PathSandbox) Tool {
	return FileStatTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileStatTool) Name() string {
	return "file.stat"
}

// Execute returns a JSON status for a sandboxed path.
func (t FileStatTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
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
		if os.IsNotExist(err) {
			return marshalStat(fileStatReply{Exists: false})
		}
		return "", fmt.Errorf("failed to access path")
	}

	reply := fileStatReply{
		Exists: true,
		IsDir:  info.IsDir(),
	}
	if !info.IsDir() {
		reply.Size = info.Size()
	}
	return marshalStat(reply)
}

type fileStatReply struct {
	Exists bool  `json:"exists"`
	IsDir  bool  `json:"isDir,omitempty"`
	Size   int64 `json:"size,omitempty"`
}

func marshalStat(reply fileStatReply) (string, error) {
	data, err := json.Marshal(reply)
	if err != nil {
		return "", fmt.Errorf("failed to render response")
	}
	return string(data), nil
}
