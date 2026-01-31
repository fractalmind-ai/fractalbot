package channels

const maxTelegramReplyChars = 3500

// TruncateTelegramReply limits outbound Telegram responses to the max Telegram message size.
func TruncateTelegramReply(text string) string {
	return truncateReply(text, maxTelegramReplyChars)
}
