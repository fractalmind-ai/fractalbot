package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/internal/config"
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
	rt := NewLoopRuntime(registry, planner, nil, 4, 0)

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
	rt := NewLoopRuntime(registry, planner, nil, 2, 0)

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
	rt := NewLoopRuntime(registry, planner, nil, 2, 0)

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
	rt := NewLoopRuntime(registry, planner, nil, 2, 0)

	reply, err := rt.HandleTask(context.Background(), Task{Text: "run", Channel: "telegram"})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if !strings.Contains(reply, "not allowed") {
		t.Fatalf("expected not allowed reply, got %q", reply)
	}
}

type contextPlanner struct{}

func (p *contextPlanner) NextStep(ctx context.Context, req PlannerRequest) (PlannerResponse, error) {
	_ = ctx
	return PlannerResponse{Reply: req.Context}, nil
}

func TestLoopRuntimePassesContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SOUL.md"), []byte("soul"), 0644); err != nil {
		t.Fatalf("write SOUL.md: %v", err)
	}
	builder, err := NewContextBuilder(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	planner := &contextPlanner{}
	rt := NewLoopRuntime(NewToolRegistry(nil), planner, builder, 2, 0)

	reply, err := rt.HandleTask(context.Background(), Task{Text: "run", Channel: "telegram"})
	if err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if !strings.Contains(reply, "SOUL.md") {
		t.Fatalf("expected context in reply, got %q", reply)
	}
}
