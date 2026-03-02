package channels

import (
	"context"
	"time"
)

// Channel defines the minimal interface for channel lifecycle management.
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	SendMessage(ctx context.Context, target string, text string) error
	IsRunning() bool
}

// SendOptions defines optional outbound message metadata.
type SendOptions struct {
	ThreadTS string
}

// ThreadedSender is implemented by channels that support threaded sends.
type ThreadedSender interface {
	SendMessageWithOptions(ctx context.Context, target string, text string, opts SendOptions) error
}

// HandlerAware is implemented by channels that accept inbound handlers.
type HandlerAware interface {
	SetHandler(handler IncomingMessageHandler)
}

// TelemetryProvider exposes channel telemetry for status reporting.
type TelemetryProvider interface {
	LastError() time.Time
	LastActivity() time.Time
}
