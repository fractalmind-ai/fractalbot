package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	fileGrepMaxBytes           = 512 * 1024
	fileGrepMaxMatches         = 50
	fileGrepMaxFiles           = 200
	fileGrepMaxLineBytes       = 200
	fileGrepTruncateNotice     = "...(truncated)"
	fileGrepLineTruncateSuffix = "..."
)

// FileGrepTool searches for substring matches within sandbox roots.
type FileGrepTool struct {
	sandbox PathSandbox
}

// NewFileGrepTool creates a new file.grep tool.
func NewFileGrepTool(sandbox PathSandbox) Tool {
	return FileGrepTool{sandbox: sandbox}
}

// Name returns the tool name.
func (FileGrepTool) Name() string {
	return "file.grep"
}

// Execute searches for matches in a file or directory.
func (t FileGrepTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	pattern, path, err := parseGrepArgs(req.Args)
	if err != nil {
		return "", err
	}
	safePath, err := t.sandbox.ValidatePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to access path")
	}

	state := &grepState{}
	baseRoot := safePath
	if info.IsDir() {
		if err := grepDirectory(safePath, baseRoot, pattern, t.sandbox, state); err != nil {
			return "", err
		}
	} else {
		baseRoot = filepath.Dir(safePath)
		if info.Size() > fileGrepMaxBytes {
			return "", fmt.Errorf("file is too large")
		}
		if err := grepFile(safePath, baseRoot, pattern, t.sandbox, state); err != nil {
			return "", err
		}
	}

	if len(state.lines) == 0 && !state.truncated {
		return "no matches found", nil
	}
	if state.truncated {
		state.lines = append(state.lines, fileGrepTruncateNotice)
	}
	return strings.Join(state.lines, "\n"), nil
}

func parseGrepArgs(args string) (string, string, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return "", "", fmt.Errorf("pattern and path are required")
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", "", fmt.Errorf("pattern and path are required")
	}
	pattern := fields[0]
	path := strings.TrimSpace(strings.Join(fields[1:], " "))
	if pattern == "" || path == "" {
		return "", "", fmt.Errorf("pattern and path are required")
	}
	return pattern, path, nil
}

type grepState struct {
	lines     []string
	matches   int
	filesSeen int
	truncated bool
}

func (s *grepState) addMatch(line string) bool {
	if s.truncated {
		return true
	}
	if s.matches >= fileGrepMaxMatches {
		s.truncated = true
		return true
	}
	s.lines = append(s.lines, line)
	s.matches++
	if s.matches >= fileGrepMaxMatches {
		s.truncated = true
	}
	return s.truncated
}

func grepDirectory(root, baseRoot, pattern string, sandbox PathSandbox, state *grepState) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("failed to read directory")
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if state.truncated {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		fullPath := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if err := grepDirectory(fullPath, baseRoot, pattern, sandbox, state); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to access file")
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if state.filesSeen >= fileGrepMaxFiles {
			state.truncated = true
			return nil
		}
		state.filesSeen++
		if info.Size() > fileGrepMaxBytes {
			continue
		}
		if err := grepFile(fullPath, baseRoot, pattern, sandbox, state); err != nil {
			return err
		}
	}
	return nil
}

func grepFile(path, baseRoot, pattern string, sandbox PathSandbox, state *grepState) error {
	safePath, err := sandbox.ValidatePath(path)
	if err != nil {
		return err
	}
	file, err := os.Open(safePath)
	if err != nil {
		return fmt.Errorf("failed to read file")
	}
	defer file.Close()

	rel, err := filepath.Rel(baseRoot, safePath)
	if err != nil {
		rel = filepath.Base(safePath)
	}
	rel = filepath.ToSlash(rel)

	reader := bufio.NewReader(file)
	lineNum := 0
	for {
		if state.truncated {
			return nil
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read file")
		}
		if len(line) > 0 {
			lineNum++
			text := strings.TrimRight(line, "\r\n")
			if strings.Contains(text, pattern) {
				text = truncateGrepLine(text)
				if state.addMatch(fmt.Sprintf("%s:%d: %s", rel, lineNum, text)) {
					return nil
				}
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

func truncateGrepLine(line string) string {
	if len(line) <= fileGrepMaxLineBytes {
		return line
	}
	suffix := fileGrepLineTruncateSuffix
	max := fileGrepMaxLineBytes - len(suffix)
	if max < 0 {
		max = 0
	}
	return line[:max] + suffix
}
