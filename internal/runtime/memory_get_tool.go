package runtime

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

const (
	memoryGetDefaultFrom  = 1
	memoryGetDefaultLines = 50
	memoryGetMaxLines     = 200
	memoryGetMaxBytes     = 256 * 1024
)

// MemoryGetTool reads a snippet from memory files under the source root.
type MemoryGetTool struct {
	cfg     *config.MemoryConfig
	sandbox PathSandbox
}

// NewMemoryGetTool creates a new memory.get tool.
func NewMemoryGetTool(cfg *config.MemoryConfig, sandbox PathSandbox) (Tool, error) {
	if cfg == nil {
		return nil, fmt.Errorf("memory config is required")
	}
	return &MemoryGetTool{cfg: cfg, sandbox: sandbox}, nil
}

// Name returns the tool name.
func (t *MemoryGetTool) Name() string {
	return "memory.get"
}

// Execute reads a slice of the memory file.
func (t *MemoryGetTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	path, from, lines, err := parseMemoryGetArgs(req.Args)
	if err != nil {
		return "", err
	}
	if err := validateMemoryGetPath(path); err != nil {
		return "", err
	}
	root := strings.TrimSpace(t.cfg.SourceRoot)
	if root == "" {
		root = "."
	}
	safeRoot, err := t.sandbox.ValidatePath(root)
	if err != nil {
		return "", err
	}
	scopedSandbox := PathSandbox{Roots: []string{safeRoot}}
	safePath, err := scopedSandbox.ValidatePath(path)
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
	out, err := readMemorySnippet(safePath, from, lines)
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}
	return out, nil
}

func parseMemoryGetArgs(args string) (string, int, int, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return "", 0, 0, fmt.Errorf("path is required")
	}
	parts := strings.SplitN(trimmed, "\n", 3)
	path := strings.TrimSpace(parts[0])
	if path == "" {
		return "", 0, 0, fmt.Errorf("path is required")
	}
	from := memoryGetDefaultFrom
	lines := memoryGetDefaultLines
	if len(parts) > 1 {
		raw := strings.TrimSpace(parts[1])
		if raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				return "", 0, 0, fmt.Errorf("invalid from line")
			}
			from = parsed
		}
	}
	if len(parts) > 2 {
		raw := strings.TrimSpace(parts[2])
		if raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				return "", 0, 0, fmt.Errorf("invalid line count")
			}
			lines = parsed
		}
	}
	if lines > memoryGetMaxLines {
		lines = memoryGetMaxLines
	}
	return path, from, lines, nil
}

func validateMemoryGetPath(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Errorf("path is required")
	}
	if filepath.IsAbs(trimmed) || isWindowsAbsPath(trimmed) {
		return fmt.Errorf("path must be relative")
	}
	if containsParentTraversal(trimmed) {
		return fmt.Errorf("path must not contain '..'")
	}
	return nil
}

func containsParentTraversal(path string) bool {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

func readMemorySnippet(path string, from, lines int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, memoryGetMaxBytes)

	end := from + lines - 1
	lineNum := 0
	var builder strings.Builder
	for scanner.Scan() {
		lineNum++
		if lineNum < from {
			continue
		}
		if lineNum > end {
			break
		}
		line := scanner.Text()
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)
		if builder.Len() > memoryGetMaxBytes {
			text := builder.String()
			return truncateToBytes(text, memoryGetMaxBytes), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func truncateToBytes(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	suffix := strings.TrimLeft(truncateSuffix, "\n")
	if max <= len(suffix) {
		return text[:max]
	}
	return text[:max-len(suffix)] + suffix
}
