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
	SendMessage(ctx context.Context, chatID int64, text string) error
	IsRunning() bool
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
