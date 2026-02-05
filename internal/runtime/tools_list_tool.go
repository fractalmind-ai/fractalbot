package runtime

import (
	"context"
	"strings"
)

// ToolsListTool returns the allowlisted tool names.
type ToolsListTool struct {
	registry *ToolRegistry
}

const noRuntimeToolsMessage = "no runtime tools are enabled (set agents.runtime.allowedTools)"

// NewToolsListTool creates a new tools.list tool.
func NewToolsListTool(registry *ToolRegistry) Tool {
	return ToolsListTool{registry: registry}
}

// Name returns the tool name.
func (ToolsListTool) Name() string {
	return "tools.list"
}

// Execute lists allowlisted tool names.
func (t ToolsListTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	_ = req
	if t.registry == nil {
		return "", nil
	}
	tools := t.registry.ListAllowedTools()
	hasTool := false
	for _, name := range tools {
		if name != "tools.list" {
			hasTool = true
			break
		}
	}
	if !hasTool {
		return noRuntimeToolsMessage, nil
	}
	return strings.Join(tools, "\n"), nil
}
