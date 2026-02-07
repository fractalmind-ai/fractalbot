package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileStatToolFileExists(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileStatTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "{\"exists\":true,\"size\":5}" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileStatToolMissingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing.txt")
	tool := NewFileStatTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "{\"exists\":false}" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileStatToolOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(outside, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileStatTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: outside}); err == nil {
		t.Fatal("expected error for outside root")
	}
}
