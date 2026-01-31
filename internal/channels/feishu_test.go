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
