package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPathSandboxAcceptsRelative(t *testing.T) {
	root := t.TempDir()
	sandbox := PathSandbox{Roots: []string{root}}
	target, err := sandbox.ValidatePath("notes.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(target, root) {
		t.Fatalf("expected target under root, got %s", target)
	}
}

func TestPathSandboxRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	sandbox := PathSandbox{Roots: []string{root}}
	if _, err := sandbox.ValidatePath("../secrets"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestPathSandboxRejectsAbsoluteOutsideRoot(t *testing.T) {
	root := t.TempDir()
	sandbox := PathSandbox{Roots: []string{root}}
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if _, err := sandbox.ValidatePath(outside); err == nil {
		t.Fatal("expected absolute path outside root to be rejected")
	}
}

func TestPathSandboxRejectsWindowsAbsolute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows absolute paths are handled by platform-specific logic")
	}
	root := t.TempDir()
	sandbox := PathSandbox{Roots: []string{root}}
	if _, err := sandbox.ValidatePath(`C:\Windows\system32`); err == nil {
		t.Fatal("expected windows absolute path to be rejected")
	}
	if _, err := sandbox.ValidatePath(`\\server\share\file`); err == nil {
		t.Fatal("expected UNC path to be rejected")
	}
}

func TestPathSandboxRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests are unreliable on Windows")
	}
	root := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linkPath := filepath.Join(root, "link")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	sandbox := PathSandbox{Roots: []string{root}}
	if _, err := sandbox.ValidatePath(linkPath); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}
