package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestBrowserCanvasToolRejectsEmptyRoots(t *testing.T) {
	tool := NewBrowserCanvasTool(PathSandbox{})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: `{"url":"https://example.com"}`}); err == nil {
		t.Fatal("expected error for empty roots")
	} else if !strings.Contains(err.Error(), "set agents.runtime.sandboxRoots") {
		t.Fatalf("expected sandboxRoots hint, got %q", err.Error())
	}
}

func TestBrowserCanvasToolRejectsNonHTTPURL(t *testing.T) {
	tool := NewBrowserCanvasTool(PathSandbox{Roots: []string{"/tmp"}})
	if _, err := tool.Execute(context.Background(), ToolRequest{Args: `{"url":"file:///tmp/test"}`}); err == nil {
		t.Fatal("expected error for non-http url")
	}
}

func TestBrowserCanvasToolReturnsStub(t *testing.T) {
	tool := NewBrowserCanvasTool(PathSandbox{Roots: []string{"/tmp"}})
	out, err := tool.Execute(context.Background(), ToolRequest{Args: `{"url":"https://example.com/path","width":800,"height":600}`})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "unsupported: browser.canvas not wired") {
		t.Fatalf("unexpected output: %q", out)
	}
	if out != "unsupported: browser.canvas not wired (host=example.com width=800 height=600)" {
		t.Fatalf("unexpected output: %q", out)
	}
}
