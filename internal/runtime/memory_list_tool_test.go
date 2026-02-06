package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestMemoryListToolHappyPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("root"), 0644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	dailyDir := filepath.Join(root, "memory", "daily")
	if err := os.MkdirAll(dailyDir, 0755); err != nil {
		t.Fatalf("mkdir daily: %v", err)
	}
	older := filepath.Join(dailyDir, "2026-01-01.md")
	newer := filepath.Join(dailyDir, "2026-01-02.md")
	if err := os.WriteFile(older, []byte("old"), 0644); err != nil {
		t.Fatalf("write older: %v", err)
	}
	if err := os.WriteFile(newer, []byte("new"), 0644); err != nil {
		t.Fatalf("write newer: %v", err)
	}
	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}
	other := filepath.Join(root, "memory", "notes.md")
	if err := os.WriteFile(other, []byte("notes"), 0644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	tool, err := NewMemoryListTool(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewMemoryListTool: %v", err)
	}
	out, err := tool.Execute(context.Background(), ToolRequest{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	expected := []string{
		"MEMORY.md",
		"memory/daily/2026-01-02.md",
		"memory/daily/2026-01-01.md",
		"memory/notes.md",
	}
	if strings.Join(expected, "\n") != out {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestMemoryListToolLimitClamp(t *testing.T) {
	root := t.TempDir()
	memoryDir := filepath.Join(root, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		t.Fatalf("mkdir memory: %v", err)
	}
	for i := 0; i < memoryListMaxLimit+10; i++ {
		path := filepath.Join(memoryDir, fmt.Sprintf("file-%03d.md", i))
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	tool, err := NewMemoryListTool(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewMemoryListTool: %v", err)
	}
	out, err := tool.Execute(context.Background(), ToolRequest{Args: "500"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != memoryListMaxLimit {
		t.Fatalf("expected %d lines, got %d", memoryListMaxLimit, len(lines))
	}
}

func TestMemoryListToolTraversalDenied(t *testing.T) {
	root := t.TempDir()
	tool, err := NewMemoryListTool(&config.MemoryConfig{SourceRoot: "../outside"}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewMemoryListTool: %v", err)
	}
	_, err = tool.Execute(context.Background(), ToolRequest{})
	if err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}
