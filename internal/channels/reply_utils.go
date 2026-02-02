package channels

import "strings"

const (
	truncateSuffix       = "\n…(truncated)"
	maxDiscordReplyChars = 1800
	maxFeishuReplyChars  = 2000
)

func truncateReply(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	return strings.TrimSpace(text[:maxChars]) + truncateSuffix
}

// TruncateFeishuReply limits outbound Feishu/Lark responses to a conservative size.
func TruncateFeishuReply(text string) string {
	return truncateReply(text, maxFeishuReplyChars)
}

// TruncateDiscordReply limits outbound Discord responses to a conservative size.
func TruncateDiscordReply(text string) string {
	return truncateReply(text, maxDiscordReplyChars)
}
