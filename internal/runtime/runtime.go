package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

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
	if memoryCfg != nil && memoryCfg.Enabled {
		sandbox := PathSandbox{Roots: cfg.SandboxRoots}
		tool, err := NewMemorySearchTool(memoryCfg, sandbox)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
	}

	maxChars := defaultMaxReplyChars
	if cfg.MaxReplyChars > 0 {
		maxChars = cfg.MaxReplyChars
	}

	return &BasicRuntime{
		registry:      registry,
		maxReplyChars: maxChars,
	}, nil
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
			return r.truncate("❌ tool name is required"), nil
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
	if strings.HasPrefix(lower, "tool:") {
		rest := strings.TrimSpace(trimmed[len("tool:"):])
		name, args := splitToolArgs(rest)
		return name, args, true
	}
	if strings.HasPrefix(lower, "tool ") {
		rest := strings.TrimSpace(trimmed[len("tool "):])
		name, args := splitToolArgs(rest)
		return name, args, true
	}
	return "", "", false
}

func splitToolArgs(rest string) (string, string) {
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", ""
	}
	name := strings.ToLower(strings.TrimSpace(fields[0]))
	args := ""
	if len(fields) > 1 {
		args = strings.Join(fields[1:], " ")
	}
	return name, strings.TrimSpace(args)
}
