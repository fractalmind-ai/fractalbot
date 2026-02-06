package runtime

import (
	"context"
	"strings"
	"testing"
)

type toolThenReplyPlanner struct{}

func (p *toolThenReplyPlanner) NextStep(ctx context.Context, task Task, events []Event) (Step, error) {
	_ = ctx
	_ = task
	for _, event := range events {
		if event.Kind == "tool_result" {
			return Step{Kind: StepKindReply, Reply: "done: " + event.Message}, nil
		}
	}
	return Step{Kind: StepKindTool, ToolName: "echo", ToolArgs: "hello"}, nil
}

type repeatToolPlanner struct {
	toolName string
	toolArgs string
}

func (p *repeatToolPlanner) NextStep(ctx context.Context, task Task, events []Event) (Step, error) {
	_ = ctx
	_ = task
	_ = events
	return Step{Kind: StepKindTool, ToolName: p.toolName, ToolArgs: p.toolArgs}, nil
}

func TestLoopRuntimeToolThenReply(t *testing.T) {
	registry := NewToolRegistry([]string{"echo"})
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	planner := &toolThenReplyPlanner{}
	rt := NewLoopRuntime(registry, planner, 4, 0)

	reply, err := rt.HandleTask(context.Background(), Task{Text: "run", Channel: "telegram"})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if reply != "done: hello" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestLoopRuntimeStepBudgetExceeded(t *testing.T) {
	registry := NewToolRegistry([]string{"echo"})
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	planner := &repeatToolPlanner{toolName: "echo", toolArgs: "hi"}
	rt := NewLoopRuntime(registry, planner, 2, 0)

	reply, err := rt.HandleTask(context.Background(), Task{Text: "run", Channel: "telegram"})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if !strings.Contains(reply, "step budget exceeded") {
		t.Fatalf("expected budget error, got %q", reply)
	}
}

func TestLoopRuntimeDisallowedTool(t *testing.T) {
	registry := NewToolRegistry(nil)
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	planner := &repeatToolPlanner{toolName: "echo", toolArgs: "hi"}
	rt := NewLoopRuntime(registry, planner, 1, 0)

	reply, err := rt.HandleTask(context.Background(), Task{Text: "run", Channel: "telegram"})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if !strings.Contains(reply, "not allowed") {
		t.Fatalf("expected not allowed reply, got %q", reply)
	}
}
