package channels

import (
	"context"
	"fmt"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeFeishuHandler struct {
	called bool
	reply  string
	err    error
	last   *protocol.Message
}

func (f *fakeFeishuHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	f.called = true
	f.last = msg
	return f.reply, f.err
}

type feishuSendCapture struct {
	receiveIDType string
	receiveID     string
	text          string
}

func TestFeishuStartStop(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_1"}, "", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	started := 0
	stopped := 0
	bot.startFn = func(ctx context.Context) error {
		started++
		return nil
	}
	bot.stopFn = func() error {
		stopped++
		return nil
	}

	if err := bot.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !bot.IsRunning() {
		t.Fatalf("expected running")
	}
	if started != 1 {
		t.Fatalf("expected start count 1, got %d", started)
	}

	if err := bot.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if bot.IsRunning() {
		t.Fatalf("expected stopped")
	}
	if stopped != 1 {
		t.Fatalf("expected stop count 1, got %d", stopped)
	}
}

func TestFeishuHelpIncludesToAlias(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", nil, "", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	text := bot.helpText()
	if !strings.Contains(text, "/to <name> <task") {
		t.Fatalf("expected help text to include /to usage")
	}
	if !strings.Contains(text, "allowlist") {
		t.Fatalf("expected help text to mention allowlist")
	}
}

func TestFeishuAllowlist(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_allowed"}, "", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	handler := &fakeFeishuHandler{reply: "ok"}
	bot.SetHandler(handler)

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("hello", "p2p", "ou_allowed", "u1", "chat1"))
	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}

	handler.called = false
	sent = feishuSendCapture{}
	bot.handleMessageEvent(context.Background(), buildFeishuEvent("hello", "p2p", "ou_blocked", "u2", "chat1"))
	if handler.called {
		t.Fatalf("expected handler not called for unauthorized user")
	}
	if !strings.Contains(sent.text, "Unauthorized") {
		t.Fatalf("expected unauthorized reply, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "ou_blocked") {
		t.Fatalf("expected open_id in reply, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "u2") {
		t.Fatalf("expected user_id in reply, got %q", sent.text)
	}
}

func TestFeishuWhoamiCommand(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", nil, "", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/whoami", "p2p", "ou_me", "u_me", "chat1"))

	if !strings.Contains(sent.text, "open_id: ou_me") {
		t.Fatalf("expected open_id, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "user_id: u_me") {
		t.Fatalf("expected user_id, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "chat_id: chat1") {
		t.Fatalf("expected chat_id, got %q", sent.text)
	}
}

func TestFeishuStatusDoesNotLeakSecret(t *testing.T) {
	bot, err := NewFeishuBot("app", "feishu-secret", "feishu", nil, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/status", "p2p", "ou_me", "u_me", "chat1"))

	if !strings.Contains(sent.text, "Bot Status") {
		t.Fatalf("expected status header, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "bot: feishu") {
		t.Fatalf("expected bot name line, got %q", sent.text)
	}
	if strings.Contains(sent.text, "feishu-secret") {
		t.Fatalf("expected status to omit secret, got %q", sent.text)
	}
}

func TestFeishuAgentsCommand(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", nil, "qa-1", []string{"qa-1", "coder-a"})
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/agents", "p2p", "ou_me", "u_me", "chat1"))

	if !strings.Contains(sent.text, "Allowed agents:") {
		t.Fatalf("expected agents header, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "Default agent: qa-1") {
		t.Fatalf("expected default agent, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "coder-a") {
		t.Fatalf("expected allowlisted agent, got %q", sent.text)
	}
	if strings.Count(sent.text, "qa-1") != 1 {
		t.Fatalf("expected default agent listed once, got %q", sent.text)
	}
}

func TestFeishuAgentsAllowlistUnset(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", nil, "qa-1", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/agents", "p2p", "ou_me", "u_me", "chat1"))

	if strings.Contains(sent.text, "No agents configured") {
		t.Fatalf("unexpected empty agents reply: %q", sent.text)
	}
	if !strings.Contains(sent.text, "Default agent: qa-1") {
		t.Fatalf("expected default agent, got %q", sent.text)
	}
	if strings.Count(sent.text, "qa-1") != 1 {
		t.Fatalf("expected default agent listed once, got %q", sent.text)
	}
}

func TestFeishuAgentsEmptyConfigHint(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", nil, "", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/agents", "p2p", "ou_me", "u_me", "chat1"))

	if !strings.Contains(sent.text, "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected defaultAgent hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "agents.ohMyCode.allowedAgents") {
		t.Fatalf("expected allowedAgents hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agent <name> <task>") {
		t.Fatalf("expected /agent hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/to <name> <task>") {
		t.Fatalf("expected /to hint, got %q", sent.text)
	}
}

func TestFeishuAgentAllowlistHint(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_allowed"}, "coder-a", []string{"coder-a"})
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/agent other test", "p2p", "ou_allowed", "u1", "chat1"))

	if !strings.Contains(sent.text, "agents.ohMyCode.allowedAgents") {
		t.Fatalf("expected allowedAgents hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agents") {
		t.Fatalf("expected /agents hint, got %q", sent.text)
	}
}

func TestFeishuAgentNotAllowedDefaultOnly(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_allowed"}, "qa-1", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/agent other test", "p2p", "ou_allowed", "u1", "chat1"))

	if !strings.Contains(sent.text, "Only the default agent is enabled") {
		t.Fatalf("expected default-only hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "agents.ohMyCode.allowedAgents") {
		t.Fatalf("expected allowedAgents hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agents") {
		t.Fatalf("expected /agents hint, got %q", sent.text)
	}
}

func TestFeishuDefaultAgentMissingGuidance(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_allowed"}, "", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("hello", "p2p", "ou_allowed", "u1", "chat1"))

	if !strings.Contains(sent.text, "agents.ohMyCode.defaultAgent") {
		t.Fatalf("expected config key hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agent <name> <task>") {
		t.Fatalf("expected /agent hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/to <name> <task>") {
		t.Fatalf("expected /to hint, got %q", sent.text)
	}
	if !strings.Contains(sent.text, "/agents") {
		t.Fatalf("expected /agents hint, got %q", sent.text)
	}
}

func TestFeishuAgentCommandUsageHint(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_allowed"}, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	for _, input := range []string{"/agent   ", "/to   "} {
		sent = feishuSendCapture{}
		bot.handleMessageEvent(context.Background(), buildFeishuEvent(input, "p2p", "ou_allowed", "u1", "chat1"))

		command := agentCommandName(input)
		if command == "" {
			command = "/agent"
		}
		expected := fmt.Sprintf("usage: %s <name> <task>", command)
		if !strings.Contains(sent.text, expected) {
			t.Fatalf("expected usage hint for %q, got %q", input, sent.text)
		}
		if !strings.Contains(sent.text, "Tip: use /agents") {
			t.Fatalf("expected /agents tip for %q, got %q", input, sent.text)
		}
	}
}

func TestFeishuAgentCommandRoutes(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_allowed"}, "", []string{"coder-a"})
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	handler := &fakeFeishuHandler{reply: "ok"}
	bot.SetHandler(handler)

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	bot.handleMessageEvent(context.Background(), buildFeishuEvent("/agent coder-a do work", "p2p", "ou_allowed", "u1", "chat1"))

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if handler.last == nil {
		t.Fatalf("expected message payload")
	}
	data, ok := handler.last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", handler.last.Data)
	}
	if data["agent"] != "coder-a" {
		t.Fatalf("expected agent coder-a, got %v", data["agent"])
	}
	if data["text"] != "do work" {
		t.Fatalf("expected task text, got %v", data["text"])
	}
	if sent.text != "ok" {
		t.Fatalf("expected reply ok, got %q", sent.text)
	}
}
func TestFeishuReplyTruncation(t *testing.T) {
	bot, err := NewFeishuBot("app", "secret", "feishu", []string{"ou_1"}, "", nil)
	if err != nil {
		t.Fatalf("NewFeishuBot: %v", err)
	}

	var sent feishuSendCapture
	bot.sendMessageFn = func(ctx context.Context, receiveIDType, receiveID, text string) error {
		sent = feishuSendCapture{receiveIDType: receiveIDType, receiveID: receiveID, text: text}
		return nil
	}

	msg := &feishuInboundMessage{replyIDType: "open_id", replyID: "ou_1"}
	longText := strings.Repeat("a", maxFeishuReplyChars+10)

	if err := bot.reply(context.Background(), msg, longText); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if !strings.Contains(sent.text, "…(truncated)") {
		t.Fatalf("expected truncated suffix, got %q", sent.text)
	}
	if len(sent.text) < maxFeishuReplyChars {
		t.Fatalf("expected truncated text length >= max, got %d", len(sent.text))
	}
}

func buildFeishuEvent(text, chatType, openID, userID, chatID string) *larkim.P2MessageReceiveV1 {
	content := fmt.Sprintf(`{"text":%q}`, text)
	return &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{
					OpenId: strPtr(openID),
					UserId: strPtr(userID),
				},
			},
			Message: &larkim.EventMessage{
				MessageId:   strPtr("msg_1"),
				ChatId:      strPtr(chatID),
				ChatType:    strPtr(chatType),
				MessageType: strPtr("text"),
				Content:     strPtr(content),
			},
		},
	}
}

func strPtr(value string) *string {
	return &value
}
