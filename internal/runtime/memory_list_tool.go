package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

const (
	memoryListDefaultLimit   = 20
	memoryListMaxLimit       = 200
	memoryListMaxOutputBytes = 64 * 1024
)

// MemoryListTool lists memory files under the configured source root.
type MemoryListTool struct {
	cfg     *config.MemoryConfig
	sandbox PathSandbox
}

// NewMemoryListTool creates a new memory.list tool.
func NewMemoryListTool(cfg *config.MemoryConfig, sandbox PathSandbox) (Tool, error) {
	if cfg == nil {
		return nil, fmt.Errorf("memory config is required")
	}
	return &MemoryListTool{cfg: cfg, sandbox: sandbox}, nil
}

// Name returns the tool name.
func (t *MemoryListTool) Name() string {
	return "memory.list"
}

// Execute lists memory files in a stable, safe order.
func (t *MemoryListTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	limit, err := parseMemoryListLimit(req.Args)
	if err != nil {
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
	info, err := os.Stat(safeRoot)
	if err != nil {
		return "", fmt.Errorf("failed to access source root")
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source root is not a directory")
	}
	items, err := collectMemoryList(safeRoot)
	if err != nil {
		return "", fmt.Errorf("failed to list memory files")
	}
	if len(items) == 0 {
		return "no memory files", nil
	}
	paths := orderMemoryList(items)
	if len(paths) > limit {
		paths = paths[:limit]
	}
	out := strings.Join(paths, "\n")
	if len(out) > memoryListMaxOutputBytes {
		out = truncateToBytes(out, memoryListMaxOutputBytes)
	}
	return out, nil
}

type memoryListItem struct {
	path    string
	modTime time.Time
	daily   bool
	isRoot  bool
}

func parseMemoryListLimit(args string) (int, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return memoryListDefaultLimit, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid limit")
	}
	if parsed > memoryListMaxLimit {
		parsed = memoryListMaxLimit
	}
	return parsed, nil
}

func collectMemoryList(root string) ([]memoryListItem, error) {
	var items []memoryListItem

	memoryFile := filepath.Join(root, "MEMORY.md")
	if info, err := os.Stat(memoryFile); err == nil && !info.IsDir() {
		items = append(items, memoryListItem{
			path:    "MEMORY.md",
			modTime: info.ModTime(),
			isRoot:  true,
		})
	}

	memoryDir := filepath.Join(root, "memory")
	if info, err := os.Stat(memoryDir); err == nil && info.IsDir() {
		err := filepath.WalkDir(memoryDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(entry.Name()) != ".md" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			items = append(items, memoryListItem{
				path:    filepath.ToSlash(rel),
				modTime: info.ModTime(),
				daily:   isDailyMemoryPath(rel),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return items, nil
}

func isDailyMemoryPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/daily/")
}

func orderMemoryList(items []memoryListItem) []string {
	var rootFile string
	var daily []memoryListItem
	var other []memoryListItem

	for _, item := range items {
		if item.isRoot {
			rootFile = item.path
			continue
		}
		if item.daily {
			daily = append(daily, item)
			continue
		}
		other = append(other, item)
	}

	sort.Slice(daily, func(i, j int) bool {
		if daily[i].modTime.Equal(daily[j].modTime) {
			return daily[i].path < daily[j].path
		}
		return daily[i].modTime.After(daily[j].modTime)
	})
	paths := make([]string, 0, len(items))
	if rootFile != "" {
		paths = append(paths, rootFile)
	}
	for _, item := range daily {
		paths = append(paths, item.path)
	}
	if len(other) > 0 {
		sort.Slice(other, func(i, j int) bool {
			return other[i].path < other[j].path
		})
		for _, item := range other {
			paths = append(paths, item.path)
		}
	}
	return paths
}
