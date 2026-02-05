package runtime

import (
	"context"
	"testing"
)

func TestToolsListToolOutputsSortedAllowlist(t *testing.T) {
	registry := NewToolRegistry([]string{"version", "echo", "tools.list", "echo", "ghost"})
	if err := registry.Register(NewVersionTool()); err != nil {
		t.Fatalf("register version: %v", err)
	}
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	if err := registry.Register(NewToolsListTool(registry)); err != nil {
		t.Fatalf("register tools.list: %v", err)
	}
	tool := NewToolsListTool(registry)
	out, err := tool.Execute(context.Background(), ToolRequest{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	expected := "echo\ntools.list\nversion"
	if out != expected {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestToolsListToolNoToolsEnabledMessage(t *testing.T) {
	registry := NewToolRegistry(nil)
	if err := registry.Register(NewToolsListTool(registry)); err != nil {
		t.Fatalf("register tools.list: %v", err)
	}
	tool := NewToolsListTool(registry)
	out, err := tool.Execute(context.Background(), ToolRequest{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != noRuntimeToolsMessage {
		t.Fatalf("unexpected output: %q", out)
	}
}
