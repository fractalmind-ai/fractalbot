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
