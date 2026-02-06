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

// ToolCall represents a planned tool invocation.
type ToolCall struct {
	Name string
	Args string
}

// ToolCallResult captures tool execution output.
type ToolCallResult struct {
	Name   string
	Output string
	Err    string
}

// PlannerRequest is the input to a planner step.
type PlannerRequest struct {
	Task           Task
	Step           int
	LastToolResult *ToolCallResult
	Context        string
}

// PlannerResponse is the output from a planner step.
type PlannerResponse struct {
	ToolCall *ToolCall
	Reply    string
}

// Planner decides the next tool call or reply.
type Planner interface {
	NextStep(ctx context.Context, req PlannerRequest) (PlannerResponse, error)
}

// LoopRuntime runs a PI-style loop with a step budget.
type LoopRuntime struct {
	registry       *ToolRegistry
	planner        Planner
	contextBuilder *ContextBuilder
	maxSteps       int
	maxReplyChars  int
	mu             sync.Mutex
	events         []Event
}

// NewLoopRuntime constructs a loop runtime with defaults.
func NewLoopRuntime(registry *ToolRegistry, planner Planner, contextBuilder *ContextBuilder, maxSteps, maxReplyChars int) *LoopRuntime {
	if maxSteps <= 0 {
		maxSteps = defaultLoopMaxSteps
	}
	if maxReplyChars <= 0 {
		maxReplyChars = defaultMaxReplyChars
	}
	return &LoopRuntime{
		registry:       registry,
		planner:        planner,
		contextBuilder: contextBuilder,
		maxSteps:       maxSteps,
		maxReplyChars:  maxReplyChars,
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

	contextText := ""
	if r.contextBuilder != nil {
		built, err := r.contextBuilder.Build(ctx)
		if err != nil {
			return "", err
		}
		contextText = built
	}

	r.emitEvent(Event{Time: time.Now().UTC(), Kind: "task_received", Agent: task.Agent, Channel: task.Channel})

	var lastResult *ToolCallResult
	for stepIndex := 0; stepIndex < r.maxSteps; stepIndex++ {
		resp, err := r.planner.NextStep(ctx, PlannerRequest{
			Task:           task,
			Step:           stepIndex + 1,
			LastToolResult: lastResult,
			Context:        contextText,
		})
		if err != nil {
			return "", err
		}
		if resp.ToolCall != nil && strings.TrimSpace(resp.Reply) != "" {
			return "", errors.New("planner returned both tool call and reply")
		}
		if resp.ToolCall != nil {
			name := strings.TrimSpace(resp.ToolCall.Name)
			if name == "" {
				return r.truncate("❌ tool name is required"), nil
			}
			result := ToolCallResult{Name: name}
			out, err := r.registry.Execute(ctx, name, ToolRequest{Args: resp.ToolCall.Args, Task: task})
			if err != nil {
				log.Printf("runtime tool error: tool=%s err=%v", name, err)
				result.Err = err.Error()
				r.emitEvent(Event{Time: time.Now().UTC(), Kind: "tool_error", Agent: task.Agent, Tool: name, Channel: task.Channel, Message: result.Err})
			} else {
				result.Output = out
				r.emitEvent(Event{Time: time.Now().UTC(), Kind: "tool_result", Agent: task.Agent, Tool: name, Channel: task.Channel, Message: out})
			}
			lastResult = &result
			continue
		}
		reply := strings.TrimSpace(resp.Reply)
		if reply == "" {
			return r.truncate("❌ planner returned no action"), nil
		}
		r.emitEvent(Event{Time: time.Now().UTC(), Kind: "step_reply", Agent: task.Agent, Channel: task.Channel, Message: reply})
		return r.truncate(reply), nil
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
