package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var agentNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)

// Config represents the main configuration.
type Config struct {
	Gateway  *GatewayConfig  `yaml:"gateway"`
	Channels *ChannelsConfig `yaml:"channels"`
	Agents   *AgentsConfig   `yaml:"agents"`
}

// GatewayConfig contains gateway settings.
type GatewayConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
	// AllowedOrigins restricts WebSocket origins. Empty means allow all.
	AllowedOrigins []string `yaml:"allowedOrigins,omitempty"`
}

// ChannelsConfig contains channel configurations.
type ChannelsConfig struct {
	Telegram *TelegramConfig `yaml:"telegram,omitempty"`
	Feishu   *FeishuConfig   `yaml:"feishu,omitempty"`
	Slack    *SlackConfig    `yaml:"slack,omitempty"`
	Discord  *DiscordConfig  `yaml:"discord,omitempty"`
	IMessage *IMessageConfig `yaml:"imessage,omitempty"`
}

// TelegramConfig contains Telegram channel settings.
type TelegramConfig struct {
	Enabled      bool    `yaml:"enabled"`
	BotToken     string  `yaml:"botToken,omitempty"`
	AllowedUsers []int64 `yaml:"allowedUsers,omitempty"`
	AllowedChats []int64 `yaml:"allowedChats,omitempty"`
	AdminID      int64   `yaml:"adminID,omitempty"`

	// Mode controls how FractalBot receives Telegram updates.
	// Supported values: "polling", "webhook". Empty means auto.
	Mode string `yaml:"mode,omitempty"`

	// PollingTimeoutSeconds is the long polling timeout used by getUpdates().
	PollingTimeoutSeconds int `yaml:"pollingTimeoutSeconds,omitempty"`
	// PollingLimit is the maximum number of updates returned per request.
	PollingLimit int `yaml:"pollingLimit,omitempty"`
	// PollingOffsetFile persists the next update offset (UpdateID+1).
	PollingOffsetFile string `yaml:"pollingOffsetFile,omitempty"`

	// WebhookListenAddr is the local bind address for receiving webhooks.
	// Example: "0.0.0.0:18790".
	WebhookListenAddr string `yaml:"webhookListenAddr,omitempty"`
	// WebhookPath is the HTTP path mounted on the webhook server.
	// Default: "/telegram/webhook".
	WebhookPath string `yaml:"webhookPath,omitempty"`
	// WebhookPublicURL is the externally reachable HTTPS URL registered with Telegram.
	WebhookPublicURL string `yaml:"webhookPublicURL,omitempty"`
	// WebhookSecretToken is verified against X-Telegram-Bot-Api-Secret-Token.
	WebhookSecretToken string `yaml:"webhookSecretToken,omitempty"`
	// WebhookRegisterOnStart controls whether FractalBot registers the webhook on startup.
	WebhookRegisterOnStart bool `yaml:"webhookRegisterOnStart,omitempty"`
	// WebhookDeleteOnStop controls whether FractalBot deletes the webhook on shutdown.
	WebhookDeleteOnStop bool `yaml:"webhookDeleteOnStop,omitempty"`
}

// FeishuConfig contains Feishu/Lark channel settings.
type FeishuConfig struct {
	Enabled bool `yaml:"enabled"`
	// AppID from Feishu/Lark developer console.
	AppID string `yaml:"appId,omitempty"`
	// AppSecret from Feishu/Lark developer console.
	AppSecret string `yaml:"appSecret,omitempty"`
	// Domain selects Feishu (China) or Lark (International).
	// Supported values: "feishu", "lark". Defaults to "feishu".
	Domain string `yaml:"domain,omitempty"`
	// AllowedUsers is an allowlist of open_id or user_id values.
	AllowedUsers []string `yaml:"allowedUsers,omitempty"`
}

// SlackConfig contains Slack channel settings.
type SlackConfig struct {
	Enabled         bool     `yaml:"enabled,omitempty"`
	BotToken        string   `yaml:"botToken,omitempty"`
	AppToken        string   `yaml:"appToken,omitempty"`
	AllowedUsers    []string `yaml:"allowedUsers,omitempty"`
	AllowedChannels []string `yaml:"allowedChannels,omitempty"`
}

// DiscordConfig contains Discord channel settings.
type DiscordConfig struct {
	Enabled      bool     `yaml:"enabled,omitempty"`
	Token        string   `yaml:"token,omitempty"`
	AllowedUsers []string `yaml:"allowedUsers,omitempty"`
}

// IMessageConfig contains iMessage channel settings.
type IMessageConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
	// Recipient is the default iMessage target (email/phone/Apple ID handle).
	Recipient string `yaml:"recipient,omitempty"`
	// Message is the fallback text used when API/CLI send text is empty.
	Message string `yaml:"message,omitempty"`
	// Service defaults to "E:iMessage" when empty.
	Service string `yaml:"service,omitempty"`
	// PollingEnabled controls whether inbound iMessage polling is enabled.
	PollingEnabled bool `yaml:"pollingEnabled,omitempty"`
	// PollingIntervalSeconds sets polling interval. Default: 5.
	PollingIntervalSeconds int `yaml:"pollingIntervalSeconds,omitempty"`
	// PollingLimit caps number of messages fetched per poll. Default: 20.
	PollingLimit int `yaml:"pollingLimit,omitempty"`
	// DatabasePath overrides Messages DB path. Default: ~/Library/Messages/chat.db.
	DatabasePath string `yaml:"databasePath,omitempty"`
}

// OhMyCodeConfig contains integration settings for the oh-my-code workspace.
// This is a minimal bridge to route Telegram messages to the oh-my-code agent-manager.
type OhMyCodeConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`

	// Workspace is the path to the oh-my-code repository.
	// Example: "/home/elliot245/workspace/elliot245/oh-my-code".
	Workspace string `yaml:"workspace,omitempty"`

	// AgentManagerScript is the path (relative to Workspace or absolute) to the agent-manager entrypoint.
	// Default: ".claude/skills/agent-manager/scripts/main.py".
	AgentManagerScript string `yaml:"agentManagerScript,omitempty"`

	// DefaultAgent is the agent name to assign tasks to when a Telegram message is received.
	// Example: "qa-1".
	DefaultAgent string `yaml:"defaultAgent,omitempty"`

	// AllowedAgents restricts which agents can be targeted by Telegram messages.
	// If empty, only DefaultAgent is allowed.
	AllowedAgents []string `yaml:"allowedAgents,omitempty"`

	// AssignTimeoutSeconds limits how long we wait for agent-manager output.
	AssignTimeoutSeconds int `yaml:"assignTimeoutSeconds,omitempty"`
}

// CodexAppCDPConfig contains routing settings for a Codex App-managed agent.
type CodexAppCDPConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`

	// CDPEndpoint is the Chromium DevTools HTTP endpoint exposed by Codex App.
	// Example: "http://127.0.0.1:9222".
	CDPEndpoint string `yaml:"cdpEndpoint,omitempty"`

	// TargetSelector matches the CDP target title or URL. Empty selects the first page target.
	TargetSelector string `yaml:"targetSelector,omitempty"`

	// HostID is the Codex App host id used by the in-app app-server manager. Defaults to "local".
	HostID string `yaml:"hostId,omitempty"`

	// ConversationID optionally pins delivery to a Codex App local conversation.
	// If empty, the CDP bridge extracts the active /local/<conversationId> route.
	ConversationID string `yaml:"conversationId,omitempty"`

	// InboxPath is a durable file-backed inbox used as the MVP queue and CDP fallback.
	InboxPath string `yaml:"inboxPath,omitempty"`

	// FallbackToInbox queues to InboxPath when CDP delivery fails.
	FallbackToInbox bool `yaml:"fallbackToInbox,omitempty"`

	// DefaultAgent is the agent name used when the inbound message omits /agent.
	DefaultAgent string `yaml:"defaultAgent,omitempty"`

	// AllowedAgents restricts which agents can be targeted by channel messages.
	// If empty, only DefaultAgent is allowed.
	AllowedAgents []string `yaml:"allowedAgents,omitempty"`

	// DeliveryTimeoutSeconds limits CDP delivery time. Defaults to 20 seconds.
	DeliveryTimeoutSeconds int `yaml:"deliveryTimeoutSeconds,omitempty"`
}

// AgentsConfig contains gateway-side agent routing settings.
type AgentsConfig struct {
	Workspace     string             `yaml:"workspace"`
	MaxConcurrent int                `yaml:"maxConcurrent"`
	Router        string             `yaml:"router,omitempty"`
	OhMyCode      *OhMyCodeConfig    `yaml:"ohMyCode,omitempty"`
	CodexAppCDP   *CodexAppCDPConfig `yaml:"codexAppCDP,omitempty"`
}

// ResolveConfigPath returns the config file path using this priority:
//  1. flagValue (if non-empty, i.e. --config was explicitly provided)
//  2. $FRACTALBOT_CONFIG environment variable
//  3. ~/.config/fractalbot/config.yaml (XDG-style default)
//  4. ./config.yaml (legacy fallback)
func ResolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}

	if env := os.Getenv("FRACTALBOT_CONFIG"); env != "" {
		return env
	}

	home, err := os.UserHomeDir()
	if err == nil {
		xdg := filepath.Join(home, ".config", "fractalbot", "config.yaml")
		if _, err := os.Stat(xdg); err == nil {
			return xdg
		}
	}

	return "./config.yaml"
}

// LoadConfig loads configuration from file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig saves configuration to file.
func SaveConfig(config *Config, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		Gateway: &GatewayConfig{
			Port: 18789,
			Bind: "127.0.0.1",
		},
		Channels: &ChannelsConfig{
			Telegram: &TelegramConfig{
				Enabled: false,
			},
		},
		Agents: &AgentsConfig{
			Workspace:     "./workspace",
			MaxConcurrent: 4,
		},
	}
}

func validateConfig(cfg *Config) error {
	if err := validateRouterConfig(cfg); err != nil {
		return err
	}
	if err := validateOhMyCodeConfig(cfg); err != nil {
		return err
	}
	if err := validateCodexAppCDPConfig(cfg); err != nil {
		return err
	}
	return nil
}

func validateRouterConfig(cfg *Config) error {
	if cfg == nil || cfg.Agents == nil {
		return nil
	}
	router := strings.TrimSpace(cfg.Agents.Router)
	if router == "" || router == "ohMyCode" || router == "codexAppCDP" {
		return nil
	}
	return fmt.Errorf("agents.router: unsupported router %q", router)
}

func validateOhMyCodeConfig(cfg *Config) error {
	if cfg == nil || cfg.Agents == nil || cfg.Agents.OhMyCode == nil {
		return nil
	}
	ohMyCode := cfg.Agents.OhMyCode
	if err := validateRoutingAgents("agents.ohMyCode", ohMyCode.DefaultAgent, ohMyCode.AllowedAgents); err != nil {
		return err
	}

	if !ohMyCode.Enabled {
		return nil
	}

	workspace := strings.TrimSpace(ohMyCode.Workspace)
	if workspace == "" {
		return fmt.Errorf("agents.ohMyCode.workspace: required when agents.ohMyCode.enabled is true")
	}

	script := strings.TrimSpace(ohMyCode.AgentManagerScript)
	if script == "" {
		return nil
	}
	if !filepath.IsAbs(workspace) {
		if filepath.IsAbs(script) {
			return fmt.Errorf("agents.ohMyCode.agentManagerScript: must be relative when agents.ohMyCode.workspace is relative")
		}
		if rel := filepath.Clean(script); rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("agents.ohMyCode.agentManagerScript: must not escape agents.ohMyCode.workspace")
		}
		return nil
	}

	resolvedScript := script
	if !filepath.IsAbs(resolvedScript) {
		resolvedScript = filepath.Join(workspace, resolvedScript)
	}
	rel, err := filepath.Rel(workspace, resolvedScript)
	if err != nil {
		return fmt.Errorf("agents.ohMyCode.agentManagerScript: must be within agents.ohMyCode.workspace")
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("agents.ohMyCode.agentManagerScript: must be within agents.ohMyCode.workspace")
	}

	return nil
}

func validateCodexAppCDPConfig(cfg *Config) error {
	if cfg == nil || cfg.Agents == nil || cfg.Agents.CodexAppCDP == nil {
		return nil
	}
	codex := cfg.Agents.CodexAppCDP
	if err := validateRoutingAgents("agents.codexAppCDP", codex.DefaultAgent, codex.AllowedAgents); err != nil {
		return err
	}
	if !codex.Enabled {
		return nil
	}
	if strings.TrimSpace(codex.InboxPath) == "" {
		return fmt.Errorf("agents.codexAppCDP.inboxPath: required when agents.codexAppCDP.enabled is true")
	}
	if codex.DeliveryTimeoutSeconds < 0 {
		return fmt.Errorf("agents.codexAppCDP.deliveryTimeoutSeconds: must be >= 0")
	}
	return nil
}

func validateRoutingAgents(prefix, defaultAgent string, allowedAgents []string) error {
	defaultAgent = strings.TrimSpace(defaultAgent)
	if defaultAgent != "" {
		if err := validateAgentName(defaultAgent); err != nil {
			return fmt.Errorf("%s.defaultAgent: %w", prefix, err)
		}
	}

	allowed := make(map[string]struct{})
	for idx, name := range allowedAgents {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return fmt.Errorf("%s.allowedAgents[%d]: agent name is required", prefix, idx)
		}
		if err := validateAgentName(trimmed); err != nil {
			return fmt.Errorf("%s.allowedAgents[%d]: %w", prefix, idx, err)
		}
		allowed[trimmed] = struct{}{}
	}

	if len(allowed) > 0 {
		if defaultAgent == "" {
			return fmt.Errorf("%s.defaultAgent: required when %s.allowedAgents is configured", prefix, prefix)
		}
		if _, ok := allowed[defaultAgent]; !ok {
			return fmt.Errorf("%s.defaultAgent: must be in %s.allowedAgents", prefix, prefix)
		}
	}
	return nil
}

func validateAgentName(name string) error {
	if !agentNamePattern.MatchString(name) {
		return fmt.Errorf("invalid agent name %q", name)
	}
	return nil
}
