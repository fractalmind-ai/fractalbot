package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileTailToolRejectsEmptyRoots(t *testing.T) {
	tool := NewFileTailTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "note.txt"}); err == nil {
		t.Fatal("expected error for empty roots")
	}
}

func TestFileTailToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(outside, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileTailTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: outside}); err == nil {
		t.Fatal("expected error for outside root")
	}
}

func TestFileTailToolRejectsDirectory(t *testing.T) {
	root := t.TempDir()
	tool := NewFileTailTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: root}); err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestFileTailToolReturnsLastLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	content := "line1\nline2\nline3\nline4\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileTailTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path + "\n2"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "line3\nline4" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFileTailToolCapsLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	var builder strings.Builder
	for i := 0; i < fileTailMaxLines+10; i++ {
		builder.WriteString(fmt.Sprintf("line-%03d\n", i))
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileTailTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path + "\n999"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != fileTailMaxLines {
		t.Fatalf("expected %d lines, got %d", fileTailMaxLines, len(lines))
	}
	if lines[0] != fmt.Sprintf("line-%03d", 10) {
		t.Fatalf("unexpected first line: %q", lines[0])
	}
}

func TestFileTailToolCapsBytes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	lineSize := len(fmt.Sprintf("line-%06d\n", 0))
	linesNeeded := (fileTailMaxBytes / lineSize) + 20
	var builder strings.Builder
	for i := 0; i < linesNeeded; i++ {
		builder.WriteString(fmt.Sprintf("line-%06d\n", i))
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tool := NewFileTailTool(PathSandbox{Roots: []string{root}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: path + "\n5"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if lines[4] != fmt.Sprintf("line-%06d", linesNeeded-1) {
		t.Fatalf("unexpected last line: %q", lines[4])
	}
}
