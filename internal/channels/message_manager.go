package channels

import "github.com/fractalmind-ai/fractalbot/pkg/protocol"

// MessageManager is a minimal stub for channel message routing.
type MessageManager struct{}

// NewMessageManager creates a new message manager.
func NewMessageManager() *MessageManager {
	return &MessageManager{}
}

// Send is a placeholder for routing channel messages.
func (m *MessageManager) Send(msg *protocol.Message) error {
	_ = msg
	return nil
}
