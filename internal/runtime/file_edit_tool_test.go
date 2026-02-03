package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileEditToolReplacesFirstOccurrence(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("hello hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	payload := `{"oldText":"hello","newText":"hi"}`
	tool := NewFileEditTool(PathSandbox{Roots: []string{root}})
	_, err := tool.Execute(context.Background(), ToolRequest{Args: target + "\n" + payload})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hi hello" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestFileEditToolRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(outside, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	payload := `{"oldText":"hello","newText":"hi"}`
	tool := NewFileEditTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: outside + "\n" + payload}); err == nil {
		t.Fatal("expected error for outside root")
	}
}

func TestFileEditToolRejectsMissingPayload(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	tool := NewFileEditTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: target}); err == nil {
		t.Fatal("expected error for missing payload")
	}
}

func TestFileEditToolRejectsOldTextMissing(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	payload := `{"oldText":"missing","newText":"hi"}`
	tool := NewFileEditTool(PathSandbox{Roots: []string{root}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: target + "\n" + payload}); err == nil {
		t.Fatal("expected error for missing oldText")
	}
}

func TestFileEditToolRejectsEmptyRoots(t *testing.T) {
	payload := `{"oldText":"hello","newText":"hi"}`
	tool := NewFileEditTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: "note.txt\n" + payload}); err == nil {
		t.Fatal("expected error for empty roots")
	}
}
