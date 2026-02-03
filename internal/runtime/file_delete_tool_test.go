package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileDeleteToolDeletesFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileDeleteTool(PathSandbox{Roots: []string{root}})
	_, err := tool.Execute(context.Background(), ToolRequest{Args: target})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, got %v", err)
	}
}

func TestFileDeleteToolRejectsDirectories(t *testing.T) {
	root := t.TempDir()
	tool := NewFileDeleteTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: root}); err == nil {
		t.Fatal("expected directory delete to be rejected")
	}
}

func TestFileDeleteToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(target, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileDeleteTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: target}); err == nil {
		t.Fatal("expected error for outside root")
	}
}

func TestFileDeleteToolRejectsEmptyRoots(t *testing.T) {
	tool := NewFileDeleteTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "note.txt"}); err == nil {
		t.Fatal("expected error for empty roots")
	}
}
