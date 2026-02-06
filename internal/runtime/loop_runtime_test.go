package runtime

import (
	"context"
	"strings"
	"testing"
)

type toolThenReplyPlanner struct{}

func (p *toolThenReplyPlanner) NextStep(ctx context.Context, req PlannerRequest) (PlannerResponse, error) {
	_ = ctx
	if req.LastToolResult == nil {
		return PlannerResponse{ToolCall: &ToolCall{Name: "echo", Args: "hello"}}, nil
	}
	return PlannerResponse{Reply: "done: " + req.LastToolResult.Output}, nil
}

type repeatToolPlanner struct {
	toolName string
	toolArgs string
}

func (p *repeatToolPlanner) NextStep(ctx context.Context, req PlannerRequest) (PlannerResponse, error) {
	_ = ctx
	_ = req
	return PlannerResponse{ToolCall: &ToolCall{Name: p.toolName, Args: p.toolArgs}}, nil
}

type directReplyPlanner struct {
	reply string
}

func (p *directReplyPlanner) NextStep(ctx context.Context, req PlannerRequest) (PlannerResponse, error) {
	_ = ctx
	_ = req
	return PlannerResponse{Reply: p.reply}, nil
}

type toolErrorPlanner struct{}

func (p *toolErrorPlanner) NextStep(ctx context.Context, req PlannerRequest) (PlannerResponse, error) {
	_ = ctx
	if req.LastToolResult != nil {
		return PlannerResponse{Reply: req.LastToolResult.Err}, nil
	}
	return PlannerResponse{ToolCall: &ToolCall{Name: "echo", Args: "hi"}}, nil
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

func TestLoopRuntimeDirectReply(t *testing.T) {
	registry := NewToolRegistry([]string{"echo"})
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	planner := &directReplyPlanner{reply: "ok"}
	rt := NewLoopRuntime(registry, planner, 2, 0)

	reply, err := rt.HandleTask(context.Background(), Task{Text: "run", Channel: "telegram"})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestLoopRuntimeDisallowedTool(t *testing.T) {
	registry := NewToolRegistry(nil)
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	planner := &toolErrorPlanner{}
	rt := NewLoopRuntime(registry, planner, 2, 0)

	reply, err := rt.HandleTask(context.Background(), Task{Text: "run", Channel: "telegram"})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if !strings.Contains(reply, "not allowed") {
		t.Fatalf("expected not allowed reply, got %q", reply)
	}
}
