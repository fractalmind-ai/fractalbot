package channels

import (
	"context"
	"errors"
	"fmt"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

// Manager starts and stops configured channels.
type Manager struct {
	cfg      *config.ChannelsConfig
	telegram *TelegramBot
}

// NewManager creates a new channel manager.
func NewManager(cfg *config.ChannelsConfig) *Manager {
	return &Manager{cfg: cfg}
}

// Start starts configured channels.
func (m *Manager) Start(ctx context.Context) error {
	if m.cfg == nil {
		return nil
	}

	if m.cfg.Telegram != nil && m.cfg.Telegram.Enabled {
		if m.cfg.Telegram.BotToken == "" {
			return errors.New("channels.telegram.botToken is required when telegram is enabled")
		}

		bot, err := NewTelegramBot(m.cfg.Telegram.BotToken, m.cfg.Telegram.AllowedUsers, m.cfg.Telegram.AdminID)
		if err != nil {
			return fmt.Errorf("failed to init telegram bot: %w", err)
		}

		bot.ConfigureWebhook(
			m.cfg.Telegram.WebhookListenAddr,
			m.cfg.Telegram.WebhookPath,
			m.cfg.Telegram.WebhookPublicURL,
			m.cfg.Telegram.WebhookSecretToken,
		)

		m.telegram = bot
		if err := bot.Start(ctx); err != nil {
			return fmt.Errorf("failed to start telegram bot: %w", err)
		}
	}

	return nil
}

// Stop stops configured channels.
func (m *Manager) Stop() error {
	var errs []error
	if m.telegram != nil {
		err := m.telegram.Stop()
		if err != nil {
			errs = append(errs, err)
		}
		m.telegram = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop channels: %v", errs)
	}

	return nil
}
