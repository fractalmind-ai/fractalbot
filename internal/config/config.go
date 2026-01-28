package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration
type Config struct {
	Gateway  *GatewayConfig  `yaml:"gateway"`
	Channels *ChannelsConfig `yaml:"channels"`
	Agents   *AgentsConfig   `yaml:"agents"`
}

// GatewayConfig contains gateway settings
type GatewayConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
}

// ChannelsConfig contains channel configurations
type ChannelsConfig struct {
	Telegram *TelegramConfig `yaml:"telegram,omitempty"`
	Slack    *SlackConfig    `yaml:"slack,omitempty"`
	Discord  *DiscordConfig  `yaml:"discord,omitempty"`
}

// TelegramConfig contains Telegram channel settings
type TelegramConfig struct {
	Enabled      bool    `yaml:"enabled"`
	BotToken     string  `yaml:"botToken,omitempty"`
	AllowedUsers []int64 `yaml:"allowedUsers,omitempty"`
}

// SlackConfig contains Slack channel settings
type SlackConfig struct {
	Enabled  bool   `yaml:"enabled,omitempty"`
	BotToken string `yaml:"botToken,omitempty"`
	AppToken string `yaml:"appToken,omitempty"`
}

// DiscordConfig contains Discord channel settings
type DiscordConfig struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Token   string `yaml:"token,omitempty"`
}

// AgentsConfig contains agent runtime settings
type AgentsConfig struct {
	Workspace     string `yaml:"workspace"`
	MaxConcurrent int    `yaml:"maxConcurrent"`
}

// LoadConfig loads configuration from file
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

// SaveConfig saves configuration to file
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

// DefaultConfig returns default configuration
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
