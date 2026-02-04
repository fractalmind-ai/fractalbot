package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestToolRegistryUnknownToolSuggestsList(t *testing.T) {
	registry := NewToolRegistry([]string{"echo"})
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	_, err := registry.Execute(context.Background(), "missing", ToolRequest{})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "tools.list") {
		t.Fatalf("expected tools.list hint, got %q", err.Error())
	}
}

func TestToolRegistryDisallowedToolSuggestsList(t *testing.T) {
	registry := NewToolRegistry([]string{"version"})
	if err := registry.Register(NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	_, err := registry.Execute(context.Background(), "echo", ToolRequest{})
	if err == nil {
		t.Fatal("expected error for disallowed tool")
	}
	if !strings.Contains(err.Error(), "tools.list") {
		t.Fatalf("expected tools.list hint, got %q", err.Error())
	}
}
