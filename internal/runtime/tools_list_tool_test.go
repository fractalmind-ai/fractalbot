package runtime

import (
	"context"
	"testing"
)

func TestToolsListToolOutputsSortedAllowlist(t *testing.T) {
	tool := NewToolsListTool([]string{"version", "echo", "tools.list", "echo"})
	out, err := tool.Execute(context.Background(), ToolRequest{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	expected := "echo\ntools.list\nversion"
	if out != expected {
		t.Fatalf("unexpected output: %q", out)
	}
}
