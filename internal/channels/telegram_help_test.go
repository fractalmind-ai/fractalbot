package channels

import (
	"strings"
	"testing"
)

func TestTelegramHelpTextIncludesAgentInfo(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 0, "qa-1", []string{"qa-1", "coder-a"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	text := bot.helpText()
	if !strings.Contains(text, "/agent <name> <task") {
		t.Fatalf("expected help text to include /agent usage")
	}
	if !strings.Contains(text, "/agents") {
		t.Fatalf("expected help text to include /agents command")
	}
	if !strings.Contains(text, "/monitor <name>") {
		t.Fatalf("expected help text to include /monitor command")
	}
	if !strings.Contains(text, "Default agent: qa-1") {
		t.Fatalf("expected help text to include default agent")
	}
	if !strings.Contains(text, "coder-a") {
		t.Fatalf("expected help text to include allowed agent")
	}
}
