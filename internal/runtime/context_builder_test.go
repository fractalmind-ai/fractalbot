package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestContextBuilderOrderingAndDailyLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SOUL.md"), []byte("soul"), 0644); err != nil {
		t.Fatalf("write SOUL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("memory"), 0644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	memoryDir := filepath.Join(root, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		t.Fatalf("mkdir memory: %v", err)
	}
	oldPath := filepath.Join(memoryDir, "2026-01-01.md")
	newPath := filepath.Join(memoryDir, "2026-01-02.md")
	thirdPath := filepath.Join(memoryDir, "2026-01-03.md")
	if err := os.WriteFile(oldPath, []byte("old"), 0644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0644); err != nil {
		t.Fatalf("write new: %v", err)
	}
	if err := os.WriteFile(thirdPath, []byte("third"), 0644); err != nil {
		t.Fatalf("write third: %v", err)
	}
	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	thirdTime := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes new: %v", err)
	}
	if err := os.Chtimes(thirdPath, thirdTime, thirdTime); err != nil {
		t.Fatalf("chtimes third: %v", err)
	}

	builder, err := NewContextBuilder(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	out, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	soulIdx := strings.Index(out, "## SOUL.md")
	memIdx := strings.Index(out, "## MEMORY.md")
	newIdx := strings.Index(out, "## memory/2026-01-03.md")
	oldIdx := strings.Index(out, "## memory/2026-01-02.md")
	missingIdx := strings.Index(out, "## memory/2026-01-01.md")
	if soulIdx == -1 || memIdx == -1 {
		t.Fatalf("expected root files in output: %q", out)
	}
	if soulIdx > memIdx {
		t.Fatalf("expected SOUL.md before MEMORY.md")
	}
	if newIdx == -1 || oldIdx == -1 {
		t.Fatalf("expected latest two daily files in output: %q", out)
	}
	if newIdx > oldIdx {
		t.Fatalf("expected newest daily file first")
	}
	if missingIdx != -1 {
		t.Fatalf("expected oldest daily file to be omitted")
	}
}

func TestContextBuilderCaps(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("a", 200)
	if err := os.WriteFile(filepath.Join(root, "SOUL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write SOUL.md: %v", err)
	}
	builder, err := NewContextBuilder(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	builder.maxFileBytes = 50
	builder.maxTotalBytes = 80
	out, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(out, "...(truncated)") {
		t.Fatalf("expected truncation marker, got %q", out)
	}
}

func TestContextBuilderTraversalRejected(t *testing.T) {
	root := t.TempDir()
	builder, err := NewContextBuilder(&config.MemoryConfig{SourceRoot: "../outside"}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	_, err = builder.Build(context.Background())
	if err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestContextBuilderSkipsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	memoryDir := filepath.Join(root, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		t.Fatalf("mkdir memory: %v", err)
	}
	linkPath := filepath.Join(memoryDir, "link.md")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	builder, err := NewContextBuilder(&config.MemoryConfig{SourceRoot: root}, PathSandbox{Roots: []string{root}})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	out, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(out, "memory/link.md") {
		t.Fatalf("expected symlink to be skipped, got %q", out)
	}
}
