package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileListToolListsDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	tool := NewFileListTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: root})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	expected := "a.txt\nb.txt\ndir/"
	if out != expected {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileListToolTruncatesOutput(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < fileListMaxEntries+5; i++ {
		name := fmt.Sprintf("file-%03d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	tool := NewFileListTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: root})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != fileListMaxEntries+1 {
		t.Fatalf("expected %d lines, got %d", fileListMaxEntries+1, len(lines))
	}
	if lines[len(lines)-1] != fileListTruncateNotice {
		t.Fatalf("expected truncate notice, got %q", lines[len(lines)-1])
	}
}

func TestFileListToolRejectsNonDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool := NewFileListTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: path}); err == nil {
		t.Fatal("expected error for non-directory path")
	}
}

func TestFileListToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	tool := NewFileListTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: outside}); err == nil {
		t.Fatal("expected error for outside root")
	}
}

func TestFileListToolRejectsEmptyRoots(t *testing.T) {
	tool := NewFileListTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "notes"}); err == nil {
		t.Fatal("expected error for empty roots")
	}
}
