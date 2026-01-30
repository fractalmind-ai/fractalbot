package channels

import (
	"context"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

// IncomingMessageHandler handles inbound channel messages and returns an optional reply.
// Channel implementations are responsible for delivering the reply back to the user.
type IncomingMessageHandler interface {
	HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error)
}

// AgentLifecycle enables channel commands to manage agent-manager processes.
type AgentLifecycle interface {
	MonitorAgent(ctx context.Context, agentName string, lines int) (string, error)
	StartAgent(ctx context.Context, agentName string) (string, error)
	StopAgent(ctx context.Context, agentName string) (string, error)
	Doctor(ctx context.Context) (string, error)
}
