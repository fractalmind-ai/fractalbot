package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestLoadConfigRejectsOhMyCodeEnabledWithoutWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    enabled: true\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.workspace") {
		t.Fatalf("expected error to mention agents.ohMyCode.workspace, got %v", err)
	}
}

func TestLoadConfigAcceptsOhMyCodeEnabledWithWorkspaceAndDefaultScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    enabled: true\n    workspace: \"/tmp\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsOhMyCodeAbsoluteScriptOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    enabled: true\n    workspace: \"" + root + "\"\n    agentManagerScript: \"/tmp/outside.py\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for script outside workspace")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.agentManagerScript") {
		t.Fatalf("expected error to mention agents.ohMyCode.agentManagerScript, got %v", err)
	}
}

func TestLoadConfigRejectsOhMyCodeRelativeScriptEscape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    enabled: true\n    workspace: \"" + root + "\"\n    agentManagerScript: \"../outside.py\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for script escaping workspace")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.agentManagerScript") {
		t.Fatalf("expected error to mention agents.ohMyCode.agentManagerScript, got %v", err)
	}
}

func TestLoadConfigRejectsOhMyCodeRelativeWorkspaceWithAbsoluteScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    enabled: true\n    workspace: \"./workspace\"\n    agentManagerScript: \"/tmp/outside.py\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for absolute script with relative workspace")
	}
	if !strings.Contains(err.Error(), "agents.ohMyCode.agentManagerScript") {
		t.Fatalf("expected error to mention agents.ohMyCode.agentManagerScript, got %v", err)
	}
}

func TestLoadConfigAcceptsOhMyCodeAbsoluteWorkspaceWithRelativeScript(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  ohMyCode:\n    enabled: true\n    workspace: \"" + root + "\"\n    agentManagerScript: \".claude/skills/agent-manager/scripts/main.py\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveConfigPathExplicitFlag(t *testing.T) {
	got := ResolveConfigPath("/custom/path.yaml")
	if got != "/custom/path.yaml" {
		t.Fatalf("expected /custom/path.yaml, got %s", got)
	}
}

func TestResolveConfigPathEnvVar(t *testing.T) {
	t.Setenv("FRACTALBOT_CONFIG", "/from/env.yaml")
	got := ResolveConfigPath("")
	if got != "/from/env.yaml" {
		t.Fatalf("expected /from/env.yaml, got %s", got)
	}
}

func TestResolveConfigPathXDGDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("FRACTALBOT_CONFIG", "")

	xdgDir := filepath.Join(dir, ".config", "fractalbot")
	if err := os.MkdirAll(xdgDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	xdgPath := filepath.Join(xdgDir, "config.yaml")
	if err := os.WriteFile(xdgPath, []byte("gateway:\n  port: 1\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := ResolveConfigPath("")
	if got != xdgPath {
		t.Fatalf("expected %s, got %s", xdgPath, got)
	}
}

func TestResolveConfigPathFallbackCWD(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("FRACTALBOT_CONFIG", "")
	// No XDG file exists, should fall back to ./config.yaml
	got := ResolveConfigPath("")
	if got != "./config.yaml" {
		t.Fatalf("expected ./config.yaml, got %s", got)
	}
}

func TestResolveConfigPathFlagTakesPriorityOverEnv(t *testing.T) {
	t.Setenv("FRACTALBOT_CONFIG", "/from/env.yaml")
	got := ResolveConfigPath("/flag/path.yaml")
	if got != "/flag/path.yaml" {
		t.Fatalf("expected /flag/path.yaml, got %s", got)
	}
}

func TestResolveConfigPathEnvTakesPriorityOverXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("FRACTALBOT_CONFIG", "/from/env.yaml")

	xdgDir := filepath.Join(dir, ".config", "fractalbot")
	if err := os.MkdirAll(xdgDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(xdgDir, "config.yaml"), []byte("gateway:\n  port: 1\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := ResolveConfigPath("")
	if got != "/from/env.yaml" {
		t.Fatalf("expected /from/env.yaml, got %s", got)
	}
}

func TestLoadConfigAcceptsIMessagePollingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`channels:
  imessage:
    enabled: true
    recipient: "recipient@example.com"
    service: "E:iMessage"
    pollingEnabled: true
    pollingIntervalSeconds: 10
    pollingLimit: 25
    databasePath: "~/Library/Messages/chat.db"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Channels == nil || cfg.Channels.IMessage == nil {
		t.Fatalf("expected channels.imessage config")
	}
	if !cfg.Channels.IMessage.PollingEnabled {
		t.Fatalf("expected pollingEnabled=true")
	}
	if cfg.Channels.IMessage.PollingIntervalSeconds != 10 {
		t.Fatalf("pollingIntervalSeconds=%d want 10", cfg.Channels.IMessage.PollingIntervalSeconds)
	}
	if cfg.Channels.IMessage.PollingLimit != 25 {
		t.Fatalf("pollingLimit=%d want 25", cfg.Channels.IMessage.PollingLimit)
	}
}

func TestLoadConfigAcceptsWeChatConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`channels:
  wechat:
    enabled: true
    provider: "wecom"
    mode: "polling"
    baseURL: "https://ilinkai.weixin.qq.com/"
    token: "bot-token-123"
    stateFile: "./workspace/wechat.polling.state.json"
    pollIntervalSeconds: 3
    callbackListenAddr: "127.0.0.1:18810"
    callbackPath: "/wechat/callback"
    callbackToken: "token-123"
    callbackEncodingAESKey: "aes-key-123"
    corpId: "ww123"
    corpSecret: "secret-123"
    agentId: "1000001"
    defaultAgent: "main"
    allowedAgents:
      - "main"
      - "qa-1"
    syncReplyTimeoutSeconds: 4
    asyncSendEnabled: true
    accessTokenCacheFile: "./workspace/wechat.token.json"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Channels == nil || cfg.Channels.WeChat == nil {
		t.Fatalf("expected channels.wechat config")
	}
	if cfg.Channels.WeChat.Provider != "wecom" {
		t.Fatalf("provider=%q want wecom", cfg.Channels.WeChat.Provider)
	}
	if cfg.Channels.WeChat.Mode != "polling" {
		t.Fatalf("mode=%q want polling", cfg.Channels.WeChat.Mode)
	}
	if cfg.Channels.WeChat.BaseURL != "https://ilinkai.weixin.qq.com/" {
		t.Fatalf("baseURL=%q", cfg.Channels.WeChat.BaseURL)
	}
	if cfg.Channels.WeChat.Token != "bot-token-123" {
		t.Fatalf("token=%q", cfg.Channels.WeChat.Token)
	}
	if cfg.Channels.WeChat.StateFile != "./workspace/wechat.polling.state.json" {
		t.Fatalf("stateFile=%q", cfg.Channels.WeChat.StateFile)
	}
	if cfg.Channels.WeChat.PollIntervalSeconds != 3 {
		t.Fatalf("pollIntervalSeconds=%d want 3", cfg.Channels.WeChat.PollIntervalSeconds)
	}
	if cfg.Channels.WeChat.CallbackPath != "/wechat/callback" {
		t.Fatalf("callbackPath=%q", cfg.Channels.WeChat.CallbackPath)
	}
	if !cfg.Channels.WeChat.AsyncSendEnabled {
		t.Fatalf("expected asyncSendEnabled=true")
	}
	if cfg.Channels.WeChat.SyncReplyTimeoutSeconds != 4 {
		t.Fatalf("syncReplyTimeoutSeconds=%d want 4", cfg.Channels.WeChat.SyncReplyTimeoutSeconds)
	}
}

func TestLoadConfigRejectsInvalidWeChatProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`channels:
  wechat:
    enabled: true
    provider: "weixin_ilink"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected invalid provider error")
	}
	if !strings.Contains(err.Error(), "channels.wechat.provider") {
		t.Fatalf("expected error to mention channels.wechat.provider, got %v", err)
	}
}

func TestLoadConfigRejectsWeChatPollingWithoutBaseURLOrToken(t *testing.T) {
	t.Run("missing baseURL", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		content := []byte(`channels:
  wechat:
    enabled: true
    provider: "wecom"
    mode: "polling"
    token: "bot-token-123"
`)
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		_, err := LoadConfig(path)
		if err == nil {
			t.Fatal("expected missing baseURL error")
		}
		if !strings.Contains(err.Error(), "channels.wechat.baseURL") {
			t.Fatalf("expected error to mention channels.wechat.baseURL, got %v", err)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		content := []byte(`channels:
  wechat:
    enabled: true
    provider: "wecom"
    mode: "polling"
    baseURL: "https://ilinkai.weixin.qq.com/"
`)
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		_, err := LoadConfig(path)
		if err == nil {
			t.Fatal("expected missing token error")
		}
		if !strings.Contains(err.Error(), "channels.wechat.token") {
			t.Fatalf("expected error to mention channels.wechat.token, got %v", err)
		}
	})
}

func TestLoadConfigRejectsWeChatAllowlistWithoutDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`channels:
  wechat:
    enabled: true
    allowedAgents:
      - "main"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected missing defaultAgent error")
	}
	if !strings.Contains(err.Error(), "channels.wechat.defaultAgent") || !strings.Contains(err.Error(), "channels.wechat.allowedAgents") {
		t.Fatalf("expected error to mention both config keys, got %v", err)
	}
}
