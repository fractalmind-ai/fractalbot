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

func TestLoadConfigRejectsInvalidDefaultAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    defaultAgent: \"-bad\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid defaultAgent")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected error to mention agents.ohMyCode.defaultAgent, got %v", err)
	}
}

func TestLoadConfigRejectsInvalidAllowedAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    allowedAgents:\n      - \"bad name\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid allowedAgents entry")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.allowedAgents[0]") {
		t.Fatalf("expected error to mention agents.ohMyCode.allowedAgents[0], got %v", err)
	}
}

func TestLoadConfigRejectsEmptyAllowedAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    allowedAgents:\n      - \"\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for empty allowedAgents entry")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.allowedAgents[0]") {
		t.Fatalf("expected error to mention agents.ohMyCode.allowedAgents[0], got %v", err)
	}
}

func TestLoadConfigRejectsAllowlistWithoutDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    allowedAgents:\n      - \"qa-1\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing defaultAgent")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.defaultAgent") || !strings.Contains(err.Error(), "agents.ohMyCode.allowedAgents") {
		t.Fatalf("expected error to mention both config keys, got %v", err)
	}
}

func TestLoadConfigRejectsDefaultNotInAllowlist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    defaultAgent: \"qa-1\"\n    allowedAgents:\n      - \"coder-a\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for defaultAgent not in allowlist")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.defaultAgent") || !strings.Contains(err.Error(), "agents.ohMyCode.allowedAgents") {
		t.Fatalf("expected error to mention both config keys, got %v", err)
	}
}

func TestLoadConfigAcceptsValidOhMyCodeAgents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    defaultAgent: \"qa-1\"\n    allowedAgents:\n      - \"qa-1\"\n      - \"coder-a\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
