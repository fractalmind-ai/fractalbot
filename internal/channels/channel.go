package channels

import "context"

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
