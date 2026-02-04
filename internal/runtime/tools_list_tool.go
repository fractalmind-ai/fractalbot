package runtime

import (
	"context"
	"sort"
	"strings"
)

// ToolsListTool returns the allowlisted tool names.
type ToolsListTool struct {
	tools []string
}

// NewToolsListTool creates a new tools.list tool.
func NewToolsListTool(allowed []string) Tool {
	return ToolsListTool{tools: normalizeToolNames(allowed)}
}

// Name returns the tool name.
func (ToolsListTool) Name() string {
	return "tools.list"
}

// Execute lists allowlisted tool names.
func (t ToolsListTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	_ = req
	if len(t.tools) == 0 {
		return "", nil
	}
	return strings.Join(t.tools, "\n"), nil
}

func normalizeToolNames(allowed []string) []string {
	unique := make(map[string]struct{})
	for _, name := range allowed {
		trimmed := strings.ToLower(strings.TrimSpace(name))
		if trimmed == "" {
			continue
		}
		unique[trimmed] = struct{}{}
	}
	out := make([]string, 0, len(unique))
	for name := range unique {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
