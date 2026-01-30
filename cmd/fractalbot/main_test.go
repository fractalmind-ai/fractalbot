package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunMissingConfig(t *testing.T) {
	var buf bytes.Buffer
	code := runWithContext(context.Background(), []string{"--config", "/nope/config.yaml"}, &buf)
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	out := buf.String()
	if !strings.Contains(out, "failed to load config") && !strings.Contains(out, "failed to read config") {
		t.Fatalf("unexpected error output: %q", out)
	}
}

func TestRunMinimalConfigExitsOnCancel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	configContents := []byte("gateway:\n  bind: 127.0.0.1\n  port: 0\n")
	if err := os.WriteFile(configPath, configContents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	var buf bytes.Buffer
	code := runWithContext(ctx, []string{"--config", configPath}, &buf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d output=%q", code, buf.String())
	}
}
