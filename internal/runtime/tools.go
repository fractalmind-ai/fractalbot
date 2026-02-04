package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Tool executes a safe, deterministic operation.
type Tool interface {
	Name() string
	Execute(ctx context.Context, req ToolRequest) (string, error)
}

// ToolRequest provides tool execution context.
type ToolRequest struct {
	Args string
	Task Task
}

// ToolRegistry dispatches tool invocations with allowlist checks.
type ToolRegistry struct {
	tools      map[string]Tool
	allowed    map[string]struct{}
	configured bool
}

// NewToolRegistry creates a registry with an allowlist.
func NewToolRegistry(allowed []string) *ToolRegistry {
	allow := make(map[string]struct{})
	for _, name := range allowed {
		trimmed := strings.ToLower(strings.TrimSpace(name))
		if trimmed == "" {
			continue
		}
		allow[trimmed] = struct{}{}
	}
	return &ToolRegistry{
		tools:      make(map[string]Tool),
		allowed:    allow,
		configured: len(allow) > 0,
	}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool Tool) error {
	if tool == nil {
		return errors.New("tool is nil")
	}
	name := strings.ToLower(strings.TrimSpace(tool.Name()))
	if name == "" {
		return errors.New("tool name is required")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// Execute runs an allowlisted tool.
func (r *ToolRegistry) Execute(ctx context.Context, name string, req ToolRequest) (string, error) {
	if r == nil {
		return "", errors.New("tool registry is nil")
	}
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return "", errors.New("tool name is required")
	}
	tool, ok := r.tools[trimmed]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", trimmed)
	}
	if !r.isAllowed(trimmed) {
		return "", fmt.Errorf("tool %q is not allowed", trimmed)
	}
	return tool.Execute(ctx, req)
}

func (r *ToolRegistry) isAllowed(name string) bool {
	if !r.configured {
		return false
	}
	_, ok := r.allowed[name]
	return ok
}

// ListAllowedTools returns sorted tool names that are both registered and allowed.
func (r *ToolRegistry) ListAllowedTools() []string {
	if r == nil || !r.configured {
		return nil
	}
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		if r.isAllowed(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// NewEchoTool returns a simple echo tool.
func NewEchoTool() Tool {
	return echoTool{}
}

type echoTool struct{}

func (echoTool) Name() string { return "echo" }

func (echoTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	return strings.TrimSpace(req.Args), nil
}

// NewVersionTool returns a fixed version tool.
func NewVersionTool() Tool {
	return versionTool{}
}

type versionTool struct{}

func (versionTool) Name() string { return "version" }

func (versionTool) Execute(ctx context.Context, req ToolRequest) (string, error) {
	_ = ctx
	_ = req
	return "runtime v0", nil
}
