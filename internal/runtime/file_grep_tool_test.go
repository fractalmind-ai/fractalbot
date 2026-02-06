package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileGrepToolRejectsEmptyRoots(t *testing.T) {
	tool := NewFileGrepTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "hit note.txt"}); err == nil {
		t.Fatal("expected error for empty roots")
	}
}

func TestFileGrepToolRejectsMissingArgs(t *testing.T) {
	root := t.TempDir()
	tool := NewFileGrepTool(PathSandbox{Roots: []string{root}})
	for _, args := range []string{"", "hit"} {
		if _, err := tool.Execute(context.Background(), ToolRequest{Args: args}); err == nil {
			t.Fatalf("expected error for args %q", args)
		}
	}
}

func TestFileGrepToolFileMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("alpha\nhit me\nomega\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileGrepTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: "hit " + path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "note.txt:2: hit me" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileGrepToolDirectoryRecursion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hit"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	subdir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "b.txt"), []byte("hit"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool := NewFileGrepTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: "hit " + root})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	expected := strings.Join([]string{
		"a.txt:1: hit",
		"sub/b.txt:1: hit",
	}, "\n")
	if out != expected {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileGrepToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(outside, []byte("hit"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileGrepTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "hit " + outside}); err == nil {
		t.Fatal("expected error for outside root")
	}
}

func TestFileGrepToolTruncatesMatches(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	var builder strings.Builder
	for i := 0; i < fileGrepMaxMatches+5; i++ {
		builder.WriteString(fmt.Sprintf("hit %d\n", i))
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool := NewFileGrepTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: "hit " + path})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != fileGrepMaxMatches+1 {
		t.Fatalf("expected %d lines, got %d", fileGrepMaxMatches+1, len(lines))
	}
	if lines[len(lines)-1] != fileGrepTruncateNotice {
		t.Fatalf("expected truncation notice, got %q", lines[len(lines)-1])
	}
}
