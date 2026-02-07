package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileExistsToolRejectsEmptyRoots(t *testing.T) {
	tool := NewFileExistsTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "note.txt"}); err == nil {
		t.Fatal("expected error for empty roots")
	}
}

func TestFileExistsToolRequiresPath(t *testing.T) {
	root := t.TempDir()
	tool := NewFileExistsTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: " "}); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestFileExistsToolReturnsTrue(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hi"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileExistsTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "true" {
		t.Fatalf("expected true, got %q", out)
	}
}

func TestFileExistsToolReturnsFalse(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing.txt")
	tool := NewFileExistsTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "false" {
		t.Fatalf("expected false, got %q", out)
	}
}

func TestFileExistsToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(outside, []byte("hi"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileExistsTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: outside}); err == nil {
		t.Fatal("expected error for outside root")
	}
}
