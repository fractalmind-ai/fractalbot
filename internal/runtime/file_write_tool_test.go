package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileWriteToolWritesFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	tool := NewFileWriteTool(PathSandbox{Roots: []string{root}})
	_, err := tool.Execute(context.Background(), ToolRequest{Args: target + "\nhello\nworld"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello\nworld" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestFileWriteToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "note.txt")
	tool := NewFileWriteTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: target + "\nhello"}); err == nil {
		t.Fatal("expected error for outside root")
	}
}

func TestFileWriteToolRejectsMissingContent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	tool := NewFileWriteTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: target}); err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestFileWriteToolRejectsEmptyRoots(t *testing.T) {
	tool := NewFileWriteTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "note.txt\nhello"}); err == nil {
		t.Fatal("expected error for empty roots")
	}
}
