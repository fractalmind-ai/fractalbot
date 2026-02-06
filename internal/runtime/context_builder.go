package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

const (
	contextMaxFileBytes  = 64 * 1024
	contextMaxTotalBytes = 256 * 1024
	contextDailyLimit    = 2
)

var contextRootFiles = []string{
	"SOUL.md",
	"USER.md",
	"TOOLS.md",
	"IDENTITY.md",
	"HEARTBEAT.md",
	"MEMORY.md",
}

// ContextBuilder assembles memory context from sourceRoot files.
type ContextBuilder struct {
	cfg           *config.MemoryConfig
	sandbox       PathSandbox
	maxFileBytes  int
	maxTotalBytes int
	dailyLimit    int
}

// NewContextBuilder creates a context builder for the memory source root.
func NewContextBuilder(cfg *config.MemoryConfig, sandbox PathSandbox) (*ContextBuilder, error) {
	if cfg == nil {
		return nil, fmt.Errorf("memory config is required")
	}
	return &ContextBuilder{
		cfg:           cfg,
		sandbox:       sandbox,
		maxFileBytes:  contextMaxFileBytes,
		maxTotalBytes: contextMaxTotalBytes,
		dailyLimit:    contextDailyLimit,
	}, nil
}

// Build assembles the context text.
func (b *ContextBuilder) Build(ctx context.Context) (string, error) {
	_ = ctx
	if b == nil {
		return "", fmt.Errorf("context builder is nil")
	}
	root := strings.TrimSpace(b.cfg.SourceRoot)
	if root == "" {
		root = "."
	}
	safeRoot, err := b.sandbox.ValidatePath(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(safeRoot)
	if err != nil {
		return "", fmt.Errorf("failed to access source root")
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source root is not a directory")
	}

	scopedSandbox := PathSandbox{Roots: []string{safeRoot}}
	sections, err := b.collectSections(scopedSandbox, safeRoot)
	if err != nil {
		return "", err
	}
	if len(sections) == 0 {
		return "", nil
	}

	output := strings.Join(sections, "\n\n")
	if b.maxTotalBytes > 0 && len(output) > b.maxTotalBytes {
		output = truncateToBytes(output, b.maxTotalBytes)
	}
	return output, nil
}

type contextSection struct {
	path string
	abs  string
}

type dailyEntry struct {
	path    string
	abs     string
	modTime time.Time
}

func (b *ContextBuilder) collectSections(sandbox PathSandbox, root string) ([]string, error) {
	sections := make([]string, 0)

	for _, name := range contextRootFiles {
		section, ok, err := b.buildSection(sandbox, root, name)
		if err != nil {
			return nil, err
		}
		if ok {
			sections = append(sections, section)
		}
	}

	daily, err := b.collectDaily(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range daily {
		section, ok, err := b.buildSection(sandbox, root, entry.path)
		if err != nil {
			return nil, err
		}
		if ok {
			sections = append(sections, section)
		}
	}

	return sections, nil
}

func (b *ContextBuilder) buildSection(sandbox PathSandbox, root, relPath string) (string, bool, error) {
	safePath, err := sandbox.ValidatePath(relPath)
	if err != nil {
		return "", false, nil
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return "", false, nil
	}
	if info.IsDir() {
		return "", false, nil
	}
	content, err := b.readFile(safePath)
	if err != nil {
		return "", false, err
	}

	return fmt.Sprintf("## %s\n%s", filepath.ToSlash(relPath), content), true, nil
}

func (b *ContextBuilder) collectDaily(root string) ([]dailyEntry, error) {
	memoryDir := filepath.Join(root, "memory")
	info, err := os.Stat(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to access memory dir")
	}
	if !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list memory dir")
	}

	var daily []dailyEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		rel := filepath.ToSlash(filepath.Join("memory", entry.Name()))
		daily = append(daily, dailyEntry{
			path:    rel,
			abs:     filepath.Join(memoryDir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	sort.Slice(daily, func(i, j int) bool {
		if daily[i].modTime.Equal(daily[j].modTime) {
			return daily[i].path < daily[j].path
		}
		return daily[i].modTime.After(daily[j].modTime)
	})

	if b.dailyLimit > 0 && len(daily) > b.dailyLimit {
		daily = daily[:b.dailyLimit]
	}
	return daily, nil
}

func (b *ContextBuilder) readFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}
	defer file.Close()

	limit := b.maxFileBytes
	if limit <= 0 {
		limit = contextMaxFileBytes
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit+1)))
	if err != nil {
		return "", fmt.Errorf("failed to read file")
	}
	content := string(data)
	if len(content) > limit {
		content = truncateToBytes(content, limit)
	}
	return strings.TrimSpace(content), nil
}
