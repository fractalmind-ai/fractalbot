package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const defaultLoopMaxSteps = 8

// StepKind describes the next action in the runtime loop.
type StepKind string

const (
	StepKindTool  StepKind = "tool"
	StepKindReply StepKind = "reply"
)

// Step represents a single loop step.
type Step struct {
	Kind     StepKind
	ToolName string
	ToolArgs string
	Reply    string
}

// Planner decides the next step based on current task and events.
type Planner interface {
	NextStep(ctx context.Context, task Task, events []Event) (Step, error)
}

// LoopRuntime runs a PI-style loop with a step budget.
type LoopRuntime struct {
	registry      *ToolRegistry
	planner       Planner
	maxSteps      int
	maxReplyChars int
	mu            sync.Mutex
	events        []Event
}

// NewLoopRuntime constructs a loop runtime with defaults.
func NewLoopRuntime(registry *ToolRegistry, planner Planner, maxSteps, maxReplyChars int) *LoopRuntime {
	if maxSteps <= 0 {
		maxSteps = defaultLoopMaxSteps
	}
	if maxReplyChars <= 0 {
		maxReplyChars = defaultMaxReplyChars
	}
	return &LoopRuntime{
		registry:      registry,
		planner:       planner,
		maxSteps:      maxSteps,
		maxReplyChars: maxReplyChars,
	}
}

// Start is a no-op for the loop runtime.
func (r *LoopRuntime) Start(ctx context.Context) error {
	_ = ctx
	return nil
}

// Stop is a no-op for the loop runtime.
func (r *LoopRuntime) Stop(ctx context.Context) error {
	_ = ctx
	return nil
}

// HandleTask executes planned steps until reply or budget is exhausted.
func (r *LoopRuntime) HandleTask(ctx context.Context, task Task) (string, error) {
	if r == nil {
		return "", errors.New("runtime is nil")
	}
	if r.registry == nil {
		return "", errors.New("runtime registry not configured")
	}
	if r.planner == nil {
		return "", errors.New("runtime planner not configured")
	}
	text := strings.TrimSpace(task.Text)
	if text == "" {
		return "", nil
	}

	r.emitEvent(Event{Time: time.Now().UTC(), Kind: "task_received", Agent: task.Agent, Channel: task.Channel})

	for stepIndex := 0; stepIndex < r.maxSteps; stepIndex++ {
		step, err := r.planner.NextStep(ctx, task, r.eventsSnapshot())
		if err != nil {
			return "", err
		}
		switch step.Kind {
		case StepKindTool:
			name := strings.TrimSpace(step.ToolName)
			if name == "" {
				return r.truncate("❌ tool name is required"), nil
			}
			out, err := r.registry.Execute(ctx, name, ToolRequest{Args: step.ToolArgs, Task: task})
			if err != nil {
				log.Printf("runtime tool error: tool=%s err=%v", name, err)
				return r.truncate(fmt.Sprintf("❌ %v", err)), nil
			}
			r.emitEvent(Event{Time: time.Now().UTC(), Kind: "tool_result", Agent: task.Agent, Tool: name, Channel: task.Channel, Message: out})
		case StepKindReply:
			reply := strings.TrimSpace(step.Reply)
			r.emitEvent(Event{Time: time.Now().UTC(), Kind: "step_reply", Agent: task.Agent, Channel: task.Channel, Message: reply})
			return r.truncate(reply), nil
		default:
			return "", fmt.Errorf("unknown step kind %q", step.Kind)
		}
	}

	return r.truncate(fmt.Sprintf("❌ step budget exceeded (%d)", r.maxSteps)), nil
}

func (r *LoopRuntime) truncate(text string) string {
	text = strings.TrimSpace(text)
	if r.maxReplyChars <= 0 || len(text) <= r.maxReplyChars {
		return text
	}
	return strings.TrimSpace(text[:r.maxReplyChars]) + truncateSuffix
}

func (r *LoopRuntime) emitEvent(event Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *LoopRuntime) eventsSnapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}
