package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSha256ToolHashesFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileSha256Tool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("unexpected hash: %q", out)
	}
}

func TestFileSha256ToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(outside, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileSha256Tool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: outside}); err == nil {
		t.Fatal("expected error for outside root")
	}
}
