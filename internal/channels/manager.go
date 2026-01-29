package channels

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

// Manager starts and stops configured channels.
type Manager struct {
	cfg       *config.ChannelsConfig
	agentsCfg *config.AgentsConfig
	handler   IncomingMessageHandler

	channels map[string]Channel
}

// NewManager creates a new channel manager.
func NewManager(cfg *config.ChannelsConfig, agentsCfg *config.AgentsConfig) *Manager {
	return &Manager{
		cfg:       cfg,
		agentsCfg: agentsCfg,
		channels:  make(map[string]Channel),
	}
}

// SetHandler sets the inbound message handler used by channels.
func (m *Manager) SetHandler(handler IncomingMessageHandler) {
	m.handler = handler
	for _, channel := range m.channels {
		if handlerAware, ok := channel.(HandlerAware); ok {
			handlerAware.SetHandler(handler)
		}
	}
}

// Register adds a channel to the manager.
func (m *Manager) Register(channel Channel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	name := channel.Name()
	if name == "" {
		return errors.New("channel name is required")
	}
	if _, exists := m.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}
	if m.handler != nil {
		if handlerAware, ok := channel.(HandlerAware); ok {
			handlerAware.SetHandler(m.handler)
		}
	}
	m.channels[name] = channel
	return nil
}

// Get returns a channel by name.
func (m *Manager) Get(name string) Channel {
	return m.channels[name]
}

// List returns registered channels in name order.
func (m *Manager) List() []Channel {
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	channels := make([]Channel, 0, len(names))
	for _, name := range names {
		channels = append(channels, m.channels[name])
	}
	return channels
}

// Start starts configured channels.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.registerConfiguredChannels(); err != nil {
		return err
	}

	for _, channel := range m.List() {
		if channel.IsRunning() {
			continue
		}
		if err := channel.Start(ctx); err != nil {
			return fmt.Errorf("failed to start %s: %w", channel.Name(), err)
		}
	}

	return nil
}

// Stop stops configured channels.
func (m *Manager) Stop() error {
	var errs []error
	for _, channel := range m.List() {
		if !channel.IsRunning() {
			continue
		}
		if err := channel.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", channel.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop channels: %v", errs)
	}

	return nil
}

func (m *Manager) registerConfiguredChannels() error {
	if m.cfg == nil {
		return nil
	}

	if m.cfg.Telegram != nil && m.cfg.Telegram.Enabled {
		if m.Get("telegram") != nil {
			return nil
		}
		if m.cfg.Telegram.BotToken == "" {
			return errors.New("channels.telegram.botToken is required when telegram is enabled")
		}

		var defaultAgent string
		var allowedAgents []string
		if m.agentsCfg != nil && m.agentsCfg.OhMyCode != nil {
			defaultAgent = m.agentsCfg.OhMyCode.DefaultAgent
			allowedAgents = m.agentsCfg.OhMyCode.AllowedAgents
		}

		bot, err := NewTelegramBot(m.cfg.Telegram.BotToken, m.cfg.Telegram.AllowedUsers, m.cfg.Telegram.AdminID, defaultAgent, allowedAgents)
		if err != nil {
			return fmt.Errorf("failed to init telegram bot: %w", err)
		}

		bot.ConfigureMode(m.cfg.Telegram.Mode)
		bot.ConfigurePolling(
			m.cfg.Telegram.PollingTimeoutSeconds,
			m.cfg.Telegram.PollingLimit,
			m.cfg.Telegram.PollingOffsetFile,
		)
		bot.ConfigureWebhook(
			m.cfg.Telegram.WebhookListenAddr,
			m.cfg.Telegram.WebhookPath,
			m.cfg.Telegram.WebhookPublicURL,
			m.cfg.Telegram.WebhookSecretToken,
		)

		if err := m.Register(bot); err != nil {
			return fmt.Errorf("failed to register telegram bot: %w", err)
		}
	}

	return nil
}
