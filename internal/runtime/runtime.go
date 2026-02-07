package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

const (
	defaultMaxReplyChars = 2000
	truncateSuffix       = "\n...(truncated)"
)

// AgentRuntime handles routed tasks and tool execution.
type AgentRuntime interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HandleTask(ctx context.Context, task Task) (string, error)
}

// Task represents a routed task with channel metadata.
type Task struct {
	Agent    string
	Text     string
	Channel  string
	Metadata map[string]string
}

// Event is a structured runtime event for future monitoring.
type Event struct {
	Time    time.Time
	Kind    string
	Agent   string
	Tool    string
	Channel string
	Message string
}

// BasicRuntime is a minimal in-process runtime.
type BasicRuntime struct {
	registry      *ToolRegistry
	eventsMu      sync.Mutex
	events        []Event
	maxReplyChars int
}

// NewRuntime creates a runtime when enabled in config.
func NewRuntime(cfg *config.RuntimeConfig, memoryCfg *config.MemoryConfig) (AgentRuntime, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	registry := NewToolRegistry(cfg.AllowedTools)
	if err := registry.Register(NewEchoTool()); err != nil {
		return nil, err
	}
	if err := registry.Register(NewVersionTool()); err != nil {
		return nil, err
	}
	if err := registry.Register(NewToolsListTool(registry)); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileReadTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileWriteTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileEditTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileDeleteTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileListTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileTailTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileExistsTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileStatTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if err := registry.Register(NewFileGrepTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	commandAllowlist := []string{}
	if cfg.CommandExec != nil {
		commandAllowlist = cfg.CommandExec.Allowlist
	}
	if err := registry.Register(NewCommandExecTool(PathSandbox{Roots: cfg.SandboxRoots}, commandAllowlist)); err != nil {
		return nil, err
	}
	if err := registry.Register(NewBrowserCanvasTool(PathSandbox{Roots: cfg.SandboxRoots})); err != nil {
		return nil, err
	}
	if memoryCfg != nil && memoryCfg.Enabled {
		sandbox := PathSandbox{Roots: cfg.SandboxRoots}
		tool, err := NewMemorySearchTool(memoryCfg, sandbox)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
		getTool, err := NewMemoryGetTool(memoryCfg, sandbox)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(getTool); err != nil {
			return nil, err
		}
		listTool, err := NewMemoryListTool(memoryCfg, sandbox)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(listTool); err != nil {
			return nil, err
		}
	}

	maxChars := defaultMaxReplyChars
	if cfg.MaxReplyChars > 0 {
		maxChars = cfg.MaxReplyChars
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "basic"
	}
	switch mode {
	case "basic":
		return &BasicRuntime{
			registry:      registry,
			maxReplyChars: maxChars,
		}, nil
	case "loop":
		var builder *ContextBuilder
		if memoryCfg != nil && memoryCfg.Enabled {
			b, err := NewContextBuilder(memoryCfg, PathSandbox{Roots: cfg.SandboxRoots})
			if err != nil {
				return nil, err
			}
			builder = b
		}
		return NewLoopRuntime(registry, &ToolCommandPlanner{}, builder, 0, maxChars), nil
	default:
		return nil, fmt.Errorf("unsupported runtime mode %q", mode)
	}
}

// Start is a no-op for the basic runtime.
func (r *BasicRuntime) Start(ctx context.Context) error {
	_ = ctx
	return nil
}

// Stop is a no-op for the basic runtime.
func (r *BasicRuntime) Stop(ctx context.Context) error {
	_ = ctx
	return nil
}

// HandleTask executes tool commands or returns a lightweight acknowledgement.
func (r *BasicRuntime) HandleTask(ctx context.Context, task Task) (string, error) {
	if r.registry == nil {
		return "", errors.New("runtime registry not configured")
	}
	text := strings.TrimSpace(task.Text)
	if text == "" {
		return "", nil
	}

	r.emitEvent(Event{
		Time:    time.Now().UTC(),
		Kind:    "task_received",
		Agent:   task.Agent,
		Channel: task.Channel,
	})

	name, args, ok := parseToolInvocation(text)
	if ok {
		if name == "" {
			return r.truncate("usage: tool <name> <args...> (see tools.list)"), nil
		}
		out, err := r.registry.Execute(ctx, name, ToolRequest{Args: args, Task: task})
		if err != nil {
			log.Printf("runtime tool error: tool=%s err=%v", name, err)
			return r.truncate(fmt.Sprintf("❌ %v", err)), nil
		}
		r.emitEvent(Event{
			Time:    time.Now().UTC(),
			Kind:    "tool_executed",
			Agent:   task.Agent,
			Tool:    name,
			Channel: task.Channel,
		})
		return r.truncate(out), nil
	}

	agentLabel := strings.TrimSpace(task.Agent)
	if agentLabel == "" {
		agentLabel = "default"
	}
	return r.truncate(fmt.Sprintf("runtime: received task for %s", agentLabel)), nil
}

func (r *BasicRuntime) emitEvent(event Event) {
	r.eventsMu.Lock()
	r.events = append(r.events, event)
	r.eventsMu.Unlock()
}

func (r *BasicRuntime) truncate(text string) string {
	text = strings.TrimSpace(text)
	if r.maxReplyChars <= 0 || len(text) <= r.maxReplyChars {
		return text
	}
	return strings.TrimSpace(text[:r.maxReplyChars]) + truncateSuffix
}

func parseToolInvocation(text string) (string, string, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	if lower == "tool" || lower == "/tool" {
		return "", "", true
	}
	if lower == "/tools" {
		return "tools.list", "", true
	}
	if strings.HasPrefix(lower, "/tools@") {
		return "tools.list", "", true
	}
	if strings.HasPrefix(lower, "tool:") {
		rest := strings.TrimSpace(trimmed[len("tool:"):])
		name, args := splitToolArgs(rest)
		return name, args, true
	}
	if strings.HasPrefix(lower, "/tool:") {
		rest := strings.TrimSpace(trimmed[len("/tool:"):])
		name, args := splitToolArgs(rest)
		return name, args, true
	}
	if strings.HasPrefix(lower, "/tool@") {
		rest := strings.TrimSpace(trimmed[len("/tool@"):])
		rest = stripToolMention(rest)
		name, args := splitToolArgs(rest)
		return name, args, true
	}
	if strings.HasPrefix(lower, "tool ") {
		rest := strings.TrimSpace(trimmed[len("tool "):])
		name, args := splitToolArgs(rest)
		return name, args, true
	}
	if strings.HasPrefix(lower, "/tool ") {
		rest := strings.TrimSpace(trimmed[len("/tool "):])
		name, args := splitToolArgs(rest)
		return name, args, true
	}
	return "", "", false
}

func splitToolArgs(rest string) (string, string) {
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return "", ""
	}
	for idx, ch := range trimmed {
		if unicode.IsSpace(ch) {
			name := strings.ToLower(strings.TrimSpace(trimmed[:idx]))
			args := strings.TrimLeftFunc(trimmed[idx:], unicode.IsSpace)
			return name, args
		}
	}
	name := strings.ToLower(strings.TrimSpace(trimmed))
	return name, ""
}

func stripToolMention(rest string) string {
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return ""
	}
	for idx, ch := range trimmed {
		if ch == ':' {
			return strings.TrimSpace(trimmed[idx+1:])
		}
		if unicode.IsSpace(ch) {
			return strings.TrimSpace(trimmed[idx+1:])
		}
	}
	return ""
}
