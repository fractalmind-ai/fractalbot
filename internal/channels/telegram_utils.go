package channels

import "strings"

const maxTelegramReplyChars = 3500

// TruncateTelegramReply limits outbound Telegram responses to the max Telegram message size.
func TruncateTelegramReply(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= maxTelegramReplyChars {
		return text
	}
	return strings.TrimSpace(text[:maxTelegramReplyChars]) + "\n…(truncated)"
}
