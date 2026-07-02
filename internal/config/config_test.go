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

func TestLoadConfigRejectsUnknownAgentRouter(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	content := []byte("agents:\n  router: rawAppServer\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected unknown router error")
	}
	if !strings.Contains(err.Error(), "agents.router") {
		t.Fatalf("expected agents.router in error, got %v", err)
	}
}

func TestLoadConfigValidatesCodexAppCDPAgentAllowlist(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	content := []byte("agents:\n  router: codexAppCDP\n  codexAppCDP:\n    enabled: true\n    inboxPath: \"" + tmp + "/inbox\"\n    defaultAgent: \"main\"\n    allowedAgents:\n      - \"main\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Agents.Router != "codexAppCDP" {
		t.Fatalf("router=%q", cfg.Agents.Router)
	}
	if cfg.Agents.CodexAppCDP == nil || cfg.Agents.CodexAppCDP.DefaultAgent != "main" {
		t.Fatalf("unexpected codexAppCDP config: %#v", cfg.Agents.CodexAppCDP)
	}
}

func TestLoadConfigRequiresCodexAppCDPEndpointOrInboxWhenEnabled(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	content := []byte("agents:\n  router: codexAppCDP\n  codexAppCDP:\n    enabled: true\n    defaultAgent: \"main\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected endpoint or inboxPath error")
	}
	if !strings.Contains(err.Error(), "agents.codexAppCDP.cdpEndpoint") || !strings.Contains(err.Error(), "agents.codexAppCDP.inboxPath") {
		t.Fatalf("expected endpoint/inboxPath error, got %v", err)
	}
}

func TestLoadConfigAcceptsCodexAppCDPReadinessConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`agents:
  router: codexAppCDP
  codexAppCDP:
    enabled: true
    cdpEndpoint: "http://127.0.0.1:9222"
    repairPolicy: "status-only"
    checkOnIncomingMessage: true
    targetProject:
      name: "CloudBank"
      cwd: "/repo/cloudbank"
      session: "main"
      stateDb: "/tmp/state.sqlite"
    watch:
      enabled: true
      intervalSeconds: 60
      cooldownSeconds: 90
    defaultAgent: "main"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Agents.CodexAppCDP == nil || cfg.Agents.CodexAppCDP.RepairPolicy != "status-only" {
		t.Fatalf("unexpected codex config: %#v", cfg.Agents.CodexAppCDP)
	}
	if cfg.Agents.CodexAppCDP.CheckOnIncomingMessage == nil || !*cfg.Agents.CodexAppCDP.CheckOnIncomingMessage {
		t.Fatalf("expected checkOnIncomingMessage=true")
	}
	if cfg.Agents.CodexAppCDP.TargetProject.Name != "CloudBank" || cfg.Agents.CodexAppCDP.TargetProject.CWD != "/repo/cloudbank" || cfg.Agents.CodexAppCDP.TargetProject.Session != "main" || cfg.Agents.CodexAppCDP.TargetProject.StateDB != "/tmp/state.sqlite" {
		t.Fatalf("unexpected targetProject: %#v", cfg.Agents.CodexAppCDP.TargetProject)
	}
	if cfg.Agents.CodexAppCDP.Watch.Enabled == nil || !*cfg.Agents.CodexAppCDP.Watch.Enabled || cfg.Agents.CodexAppCDP.Watch.IntervalSeconds != 60 || cfg.Agents.CodexAppCDP.Watch.CooldownSeconds != 90 {
		t.Fatalf("unexpected watch config: %#v", cfg.Agents.CodexAppCDP.Watch)
	}
}

func TestLoadConfigRejectsInvalidCodexAppCDPRepairPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("agents:\n  router: codexAppCDP\n  codexAppCDP:\n    enabled: true\n    cdpEndpoint: \"http://127.0.0.1:9222\"\n    repairPolicy: \"restart-everything\"\n    defaultAgent: \"main\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected repairPolicy error")
	}
	if !strings.Contains(err.Error(), "agents.codexAppCDP.repairPolicy") {
		t.Fatalf("expected repairPolicy error, got %v", err)
	}
}

func TestLoadConfigParsesDemailChannel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(strings.Join([]string{
		"channels:",
		"  demail:",
		"    enabled: true",
		"    rpcUrl: \"https://fullnode.testnet.sui.io:443\"",
		"    packageId: \"0xpkg\"",
		"    address: \"0xabc\"",
		"    identityKeyFile: \"/tmp/identity.key\"",
		"    sponsorAddress: \"0xdef\"",
		"    gasCoin: \"0xgas\"",
		"    pollIntervalSeconds: 7",
		"    cursorFile: \"/tmp/demail-cursor.log\"",
		"    allowedSenders:",
		"      - \"0xpeer\"",
		"    peers:",
		"      \"0xpeer\": \"cHVibGljLWtleQ==\"",
		"",
	}, "\n"))
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Channels == nil || cfg.Channels.Demail == nil {
		t.Fatal("expected channels.demail to be parsed")
	}
	demail := cfg.Channels.Demail
	if !demail.Enabled {
		t.Fatal("expected demail.enabled true")
	}
	if demail.RPCURL != "https://fullnode.testnet.sui.io:443" {
		t.Fatalf("unexpected rpcUrl: %q", demail.RPCURL)
	}
	if demail.PackageID != "0xpkg" || demail.Address != "0xabc" {
		t.Fatalf("unexpected packageId/address: %q %q", demail.PackageID, demail.Address)
	}
	if demail.IdentityKeyFile != "/tmp/identity.key" {
		t.Fatalf("unexpected identityKeyFile: %q", demail.IdentityKeyFile)
	}
	if demail.SponsorAddress != "0xdef" || demail.GasCoin != "0xgas" {
		t.Fatalf("unexpected sponsorAddress/gasCoin: %q %q", demail.SponsorAddress, demail.GasCoin)
	}
	if demail.PollIntervalSeconds != 7 {
		t.Fatalf("unexpected pollIntervalSeconds: %d", demail.PollIntervalSeconds)
	}
	if demail.CursorFile != "/tmp/demail-cursor.log" {
		t.Fatalf("unexpected cursorFile: %q", demail.CursorFile)
	}
	if len(demail.AllowedSenders) != 1 || demail.AllowedSenders[0] != "0xpeer" {
		t.Fatalf("unexpected allowedSenders: %v", demail.AllowedSenders)
	}
	if demail.Peers["0xpeer"] != "cHVibGljLWtleQ==" {
		t.Fatalf("unexpected peers: %v", demail.Peers)
	}
}

func TestLoadConfigDemailDisabledByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("channels:\n  telegram:\n    enabled: false\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Channels.Demail != nil {
		t.Fatalf("expected nil demail config, got %#v", cfg.Channels.Demail)
	}
}
