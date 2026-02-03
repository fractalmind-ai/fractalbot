package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	fileListMaxEntries     = 200
	fileListTruncateNotice = "...(truncated)"
)

// FileListTool lists directory entries within sandbox roots.
type FileListTool struct {
	sandbox PathSandbox
}

// NewFileListTool creates a new file.list tool.
func NewFileListTool(sandbox PathSandbox) Tool {
	return FileListTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileListTool) Name() string {
	return "file.list"
}

// Execute lists a sandboxed directory.
func (t FileListTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
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
		return "", fmt.Errorf("failed to access directory")
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}
	entries, err := os.ReadDir(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to list directory")
	}

	items := make([]fileListEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, fileListEntry{name: entry.Name(), isDir: entry.IsDir()})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].name < items[j].name
	})

	truncated := false
	if len(items) > fileListMaxEntries {
		items = items[:fileListMaxEntries]
		truncated = true
	}

	lines := make([]string, 0, len(items)+1)
	for _, item := range items {
		name := item.name
		if item.isDir {
			name += "/"
		}
		lines = append(lines, name)
	}
	if truncated {
		lines = append(lines, fileListTruncateNotice)
	}
	return strings.Join(lines, "\n"), nil
}

type fileListEntry struct {
	name  string
	isDir bool
}
