package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestMemoryGetToolHappyPath(t *testing.T) {
	root := t.TempDir()
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool, err := NewMemoryGetTool(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewMemoryGetTool: %v", err)
	}
	out, err := tool.Execute(context.Background(), ToolRequest{Args: "MEMORY.md\n2\n2"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	expected := "line2\nline3"
	if out != expected {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestMemoryGetToolDefaultsAndClamp(t *testing.T) {
	root := t.TempDir()
	var builder strings.Builder
	for i := 1; i <= 210; i++ {
		builder.WriteString(fmt.Sprintf("line%d\n", i))
	}
	if err := os.WriteFile(filepath.Join(root, "memory.md"), []byte(builder.String()), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool, err := NewMemoryGetTool(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewMemoryGetTool: %v", err)
	}
	out, err := tool.Execute(context.Background(), ToolRequest{Args: "memory.md"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != memoryGetDefaultLines {
		t.Fatalf("expected %d default lines, got %d", memoryGetDefaultLines, len(lines))
	}

	out, err = tool.Execute(context.Background(), ToolRequest{Args: "memory.md\n1\n500"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines = strings.Split(out, "\n")
	if len(lines) != memoryGetMaxLines {
		t.Fatalf("expected %d lines, got %d", memoryGetMaxLines, len(lines))
	}
}

func TestMemoryGetToolTraversalDenied(t *testing.T) {
	root := t.TempDir()
	tool, err := NewMemoryGetTool(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewMemoryGetTool: %v", err)
	}
	_, err = tool.Execute(context.Background(), ToolRequest{Args: "../secrets.txt"})
	if err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Fatalf("expected traversal error, got %v", err)
	}
}
