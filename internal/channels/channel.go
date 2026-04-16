package channels

import (
	"context"
	"time"
)

// Channel defines the minimal interface for channel lifecycle management.
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg OutboundMessage) (*SendResult, error)
	IsRunning() bool
	IsAllowed(senderID string) bool
}

// SendResult carries metadata about a successfully sent message.
// Fields are best-effort: channels populate what they can.
type SendResult struct {
	ChannelID   string `json:"channel_id,omitempty"`
	ChannelName string `json:"channel_name,omitempty"`
	MessageTS   string `json:"message_ts,omitempty"`
	ThreadTS    string `json:"thread_ts,omitempty"`
}

// OutboundMessage carries all data needed to send a message through a channel.
type OutboundMessage struct {
	To       string
	Text     string
	ThreadTS string // thread/reply context (Slack threads, etc.)
}

// MediaPart represents a single media attachment for outbound messages.
type MediaPart struct {
	Type     string // "image", "video", "audio", "file"
	Filename string
	URL      string
	MimeType string
	Data     []byte // inline upload data (optional)
}

// TypingCapable is implemented by channels that support typing indicators.
type TypingCapable interface {
	StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

// MessageEditor is implemented by channels that support editing sent messages.
type MessageEditor interface {
	EditMessage(ctx context.Context, chatID, messageID, content string) error
}

// ReactionCapable is implemented by channels that support message reactions.
type ReactionCapable interface {
	AddReaction(ctx context.Context, chatID, messageID, emoji string) (undo func(), err error)
}

// PlaceholderCapable is implemented by channels that support placeholder messages.
type PlaceholderCapable interface {
	SendPlaceholder(ctx context.Context, chatID, text string) (messageID string, err error)
}

// MediaSender is implemented by channels that support sending media attachments.
type MediaSender interface {
	SendMedia(ctx context.Context, chatID string, parts []MediaPart) error
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
