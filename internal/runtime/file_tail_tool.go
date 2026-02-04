package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	fileTailDefaultLines = 50
	fileTailMaxLines     = 200
	fileTailMaxBytes     = 64 * 1024
)

// FileTailTool reads the last N lines from a file within sandbox roots.
type FileTailTool struct {
	sandbox PathSandbox
}

// NewFileTailTool creates a new file.tail tool.
func NewFileTailTool(sandbox PathSandbox) Tool {
	return FileTailTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileTailTool) Name() string {
	return "file.tail"
}

// Execute reads the last N lines from a sandboxed file path.
func (t FileTailTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	path, lines, err := parseTailArgs(req.Args)
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
	out, err := tailFile(safePath, lines)
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}
	return out, nil
}

func parseTailArgs(args string) (string, int, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return "", 0, fmt.Errorf("path is required")
	}
	lines := strings.SplitN(trimmed, "\n", 2)
	path := strings.TrimSpace(lines[0])
	if path == "" {
		return "", 0, fmt.Errorf("path is required")
	}
	count := fileTailDefaultLines
	if len(lines) > 1 {
		raw := strings.TrimSpace(lines[1])
		if raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				return "", 0, fmt.Errorf("invalid line count")
			}
			count = parsed
		}
	}
	if count > fileTailMaxLines {
		count = fileTailMaxLines
	}
	return path, count, nil
}

func tailFile(path string, lines int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()
	if size <= 0 {
		return "", nil
	}
	readSize := size
	if readSize > fileTailMaxBytes {
		readSize = fileTailMaxBytes
	}
	offset := size - readSize

	buf := make([]byte, int(readSize))
	reader := io.NewSectionReader(file, offset, readSize)
	if _, err := io.ReadFull(reader, buf); err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}

	text := string(buf)
	parts := strings.Split(text, "\n")
	if offset > 0 && len(parts) > 1 {
		parts = parts[1:]
	}
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return "", nil
	}
	if lines > len(parts) {
		lines = len(parts)
	}
	return strings.Join(parts[len(parts)-lines:], "\n"), nil
}
