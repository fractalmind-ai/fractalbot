package channels

import (
	"context"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

// Manager is a minimal channel manager stub.
type Manager struct{}

// NewManager creates a new channel manager.
func NewManager(cfg *config.ChannelsConfig) *Manager {
	_ = cfg
	return &Manager{}
}

// Start starts configured channels.
func (m *Manager) Start(ctx context.Context) error {
	_ = ctx
	return nil
}

// Stop stops configured channels.
func (m *Manager) Stop() error {
	return nil
}
