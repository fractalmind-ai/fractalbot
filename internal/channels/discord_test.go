package channels

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeDiscordHandler struct {
	called bool
	reply  string
	err    error
	last   *protocol.Message
}

func (f *fakeDiscordHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	f.called = true
	f.last = msg
	return f.reply, f.err
}

type discordSendCapture struct {
	channelID string
	text      string
}

func TestDiscordAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "hello",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}
	if handler.last == nil {
		t.Fatalf("expected protocol message")
	}
	data, ok := handler.last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", handler.last.Data)
	}
	if data["channel"] != "discord" {
		t.Fatalf("expected channel discord, got %v", data["channel"])
	}
}

func TestDiscordUnauthorized(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "hello",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if handler.called {
		t.Fatalf("expected handler not called for unauthorized user")
	}
	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "999") {
		t.Fatalf("expected user ID in reply, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "channels.discord.allowedUsers") {
		t.Fatalf("expected allowedUsers hint in reply, got %q", sent.text)
	}
}

func TestDiscordWhoamiAllowedWithoutAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/whoami",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "user_id: 999") {
		t.Fatalf("expected whoami reply, got %q", sent.text)
	}
	if strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("did not expect unauthorized for /whoami, got %q", sent.text)
	}
}

func TestDiscordAgentsEmptyConfigHint(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/agents",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected defaultAgent hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "agents.ohMyCode.allowedAgents") {
		t.Fatalf("expected allowedAgents hint, got %q", sent.text)
	}
}

func TestDiscordAgentsDedupesDefault(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "qa-1", []string{"qa-1", "coder-a"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/agents",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "Default agent: qa-1") {
		t.Fatalf("expected default agent line, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "coder-a") {
		t.Fatalf("expected allowlisted agent, got %q", sent.text)
	}
	if strings.Count(sent.text, "qa-1") != 1 {
		t.Fatalf("expected default agent listed once, got %q", sent.text)
	}
}

func TestDiscordIgnoreNonDM(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "hello",
		userID:      "123",
		channelID:   "C123",
		channelType: "channel",
	})

	if handler.called {
		t.Fatalf("expected handler not called for non-DM")
	}
	if sent.text != "" {
		t.Fatalf("expected no reply for non-DM, got %q", sent.text)
	}
}

func TestDiscordMessageFromEventFiltersGuild(t *testing.T) {
	msg := discordMessageFromEvent(&discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "m1",
			ChannelID: "c1",
			GuildID:   "g1",
			Content:   "hi",
			Author:    &discordgo.User{ID: "u1"},
		},
	})
	if msg == nil {
		t.Fatalf("expected message for non-bot event")
	}
	if msg.channelType != "guild" {
		t.Fatalf("expected guild channel type, got %q", msg.channelType)
	}
}

func TestDiscordReplyTruncation(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: strings.Repeat("a", maxDiscordReplyChars+10)}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "hello",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "…(truncated)") {
		t.Fatalf("expected truncated reply, got %q", sent.text)
	}
}

func TestDiscordStatusDoesNotLeakTokens(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/status",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "Bot Status") {
		t.Fatalf("expected status header, got %q", sent.text)
	}
	if strings.Contains(sent.text, "discord-secret") {
		t.Fatalf("expected status to omit token, got %q", sent.text)
	}
}

func TestDiscordStatusAllowedWithoutAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/status",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "Bot Status") {
		t.Fatalf("expected status header, got %q", sent.text)
	}
	if strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("did not expect unauthorized for /status, got %q", sent.text)
	}
}

func TestDiscordIncompleteAgentUsageBypassesAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	tests := []string{
		"/agent",
		"/agent qa-1",
		"/agent@bot",
		"/agent@bot qa-1",
		"/to",
		"/to qa-1",
		"/to@bot",
		"/to@bot qa-1",
	}
	for _, input := range tests {
		var sent discordSendCapture
		bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
			_ = ctx
			sent = discordSendCapture{channelID: channelID, text: text}
			return nil
		}

		bot.handleMessageEvent(context.Background(), &discordInboundMessage{
			text:        input,
			userID:      "999",
			channelID:   "D123",
			channelType: "dm",
		})

		if !strings.Contains(sent.text, "usage: /agent <name> <task...>") {
			t.Fatalf("expected usage hint for %q, got %q", input, sent.text)
		}
		if strings.Contains(sent.text, "Unauthorized") {
			t.Fatalf("did not expect unauthorized for %q, got %q", input, sent.text)
		}
	}
}

func TestDiscordAgentWithTaskStillUnauthorized(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/agent qa-1 hello",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if handler.called {
		t.Fatalf("expected handler not called for unauthorized user")
	}
	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
}

func TestDiscordHelpIncludesToAlias(t *testing.T) {
	bot, err := NewDiscordBot("token", nil, "", nil)
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	text := bot.helpText()
	if !strings.Contains(text, "/to <name> <task") {
		t.Fatalf("expected help text to include /to usage")
	}
}

func TestDiscordToolsAllowedWithoutAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	handler := &fakeDiscordHandler{reply: "⚠️ runtime tools are disabled"}
	bot.SetHandler(handler)

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/tools",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("did not expect unauthorized for /tools, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "runtime tools are disabled") {
		t.Fatalf("expected tools response, got %q", sent.text)
	}
}

func TestDiscordToolCommandUnauthorized(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/tool: echo hi",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
}

func TestDiscordToolCommandAllowedRoutes(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	handler := &fakeDiscordHandler{reply: "hi"}
	bot.SetHandler(handler)

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/tool: echo hi",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
	})

	if sent.text != "hi" {
		t.Fatalf("expected reply hi, got %q", sent.text)
	}
}

func TestDiscordStatusWithMentionBypassesAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/status@bot",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if !strings.Contains(sent.text, "Bot Status") {
		t.Fatalf("expected status header, got %q", sent.text)
	}
	if strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("did not expect unauthorized for /status@bot, got %q", sent.text)
	}
}

func TestDiscordAgentWithMentionRoutes(t *testing.T) {
	bot, err := NewDiscordBot("token", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	handler := &fakeDiscordHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/agent@bot qa-1 hello",
		userID:      "123",
		channelID:   "D123",
		channelType: "dm",
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if handler.last == nil {
		t.Fatalf("expected protocol message")
	}
	data, ok := handler.last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", handler.last.Data)
	}
	if data["text"] != "hello" {
		t.Fatalf("expected task text, got %v", data["text"])
	}
}

func TestDiscordAgentsAllowedWithoutAllowlist(t *testing.T) {
	bot, err := NewDiscordBot("discord-secret", []string{"123"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewDiscordBot: %v", err)
	}

	var sent discordSendCapture
	bot.sendMessageFn = func(ctx context.Context, channelID, text string) error {
		_ = ctx
		sent = discordSendCapture{channelID: channelID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), &discordInboundMessage{
		text:        "/agents",
		userID:      "999",
		channelID:   "D123",
		channelType: "dm",
	})

	if strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("did not expect unauthorized for /agents, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "Allowed agents:") && !strings.Contains(sent.text, "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected agents output or config hint, got %q", sent.text)
	}
}
