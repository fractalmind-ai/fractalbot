package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigRejectsInvalidMemoryModelID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  memory:\n    modelId: \"../bad\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid modelId")
	}
	if !strings.Contains(err.Error(), "agents.memory.modelId") {
		t.Fatalf("expected error to mention agents.memory.modelId, got %v", err)
	}
}

func TestLoadConfigAcceptsValidMemoryModelID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  memory:\n    modelId: \"e5_small.v2\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
