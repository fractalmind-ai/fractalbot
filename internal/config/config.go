package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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
}

// TelegramConfig contains Telegram channel settings.
type TelegramConfig struct {
	Enabled      bool    `yaml:"enabled"`
	BotToken     string  `yaml:"botToken,omitempty"`
	AllowedUsers []int64 `yaml:"allowedUsers,omitempty"`
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
	Enabled  bool   `yaml:"enabled,omitempty"`
	BotToken string `yaml:"botToken,omitempty"`
	AppToken string `yaml:"appToken,omitempty"`
}

// DiscordConfig contains Discord channel settings.
type DiscordConfig struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Token   string `yaml:"token,omitempty"`
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

// AgentsConfig contains agent runtime settings.
type AgentsConfig struct {
	Workspace     string          `yaml:"workspace"`
	MaxConcurrent int             `yaml:"maxConcurrent"`
	OhMyCode      *OhMyCodeConfig `yaml:"ohMyCode,omitempty"`
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
