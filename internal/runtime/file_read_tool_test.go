package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileReadToolReadsFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileReadTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "hello" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileReadToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileReadTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: outside}); err == nil {
		t.Fatal("expected error for outside root")
	}
}
