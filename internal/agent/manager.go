package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/fractalmind-ai/fractalbot/internal/channels"
	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

// Manager is a minimal stub for agent lifecycle management.
type Manager struct {
	config         *config.AgentsConfig
	ChannelManager *channels.Manager
	mu             sync.RWMutex
	agents         map[string]protocol.AgentInfo
}

// NewManager creates a new agent manager.
func NewManager(cfg *config.AgentsConfig) *Manager {
	return &Manager{
		config: cfg,
		agents: make(map[string]protocol.AgentInfo),
	}
}

// Start initializes agent runtime.
func (m *Manager) Start(ctx context.Context) error {
	_ = ctx
	return nil
}

// Stop halts agent runtime.
func (m *Manager) Stop(ctx context.Context) error {
	_ = ctx
	return nil
}

// List returns known agents.
func (m *Manager) List() []protocol.AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]protocol.AgentInfo, 0, len(m.agents))
	for _, info := range m.agents {
		agents = append(agents, info)
	}
	return agents
}

// HandleIncoming implements channels.IncomingMessageHandler.
func (m *Manager) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	_ = ctx
	if msg == nil {
		return "", nil
	}

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		return "", nil
	}

	channel, _ := data["channel"].(string)
	if channel != "telegram" {
		return "", nil
	}

	text, _ := data["text"].(string)
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}

	return fmt.Sprintf("echo: %s", text), nil
}
