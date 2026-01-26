package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

// TelegramBot implements Telegram channel
type TelegramBot struct {
	botToken   string
	manager    *MessageManager
	allowedIDs map[int64]bool
	webhookURL string
	webhookSecret string
	server     *http.Server
	stopChan   chan struct{}
	ctx         context.Context
}

// NewTelegramBot creates a new Telegram bot instance
func NewTelegramBot(token string, allowedUsers []int64) (*TelegramBot, error) {
	allowedIDs := make(map[int64]bool)
	for _, id := range allowedUsers {
		allowedIDs[id] = true
	}

	return &TelegramBot{
		botToken:      token,
		manager:       NewMessageManager(),
		allowedIDs:     allowedIDs,
		webhookURL:     "",
		webhookSecret: "",
		server:        nil,
		stopChan:       make(chan struct{}),
		ctx:            context.Background(),
	}, nil
}

// Name returns the bot name
func (b *TelegramBot) Name() string {
	return "telegram"
}

// Start begins webhook server
func (b *TelegramBot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	log.Println("📱 Telegram bot starting...")

	// Start webhook server (if configured)
	if b.webhookURL != "" && b.webhookSecret != "" {
		b.server = &http.Server{
			Addr:    b.webhookURL,
			Handler:  http.HandlerFunc(b.handleWebhook),
			ErrorLog: log.New(io.Discard, "", log.LstdFlags),
		}

		go func() {
			log.Printf("📡 Webhook server listening on %s", b.webhookURL)
			if err := b.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Webhook server error: %v", err)
			}
		}()
	}

	return nil
}

// SetWebhook configures webhook settings
func (b *TelegramBot) SetWebhook(url, secret string) {
	b.webhookURL = url
	b.webhookSecret = secret
	log.Printf("🔗 Telegram webhook configured: %s", url)
}

// Stop gracefully shuts down the bot
func (b *TelegramBot) Stop() error {
	log.Println("🛑 Stopping Telegram bot...")

	close(b.stopChan)
	b.cancel()

	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		b.server.Shutdown(ctx)
	}

	return nil
}

// handleWebhook handles incoming Telegram webhook updates
func (b *TelegramBot) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// TODO: Verify webhook secret

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read webhook body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var update struct {
		UpdateID int64         `json:"update_id"`
		Message  *TelegramMessage `json:"message"`
	}

	if err := json.Unmarshal(body, &update); err != nil {
		log.Printf("Failed to parse webhook update: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check if it's a message
	if update.Message == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check if user is allowed
	if !b.isAllowed(update.Message.From.ID) {
		log.Printf("🚫 Unauthorized access attempt: User ID %d", update.Message.From.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Convert to protocol message
	msg := b.convertToProtocolMessage(update.Message)

	// Send to manager
	if err := b.manager.Send(msg); err != nil {
		log.Printf("Error sending to message manager: %v", err)
	}
}

// TelegramMessage represents a Telegram message
type TelegramMessage struct {
	MessageID int64   `json:"message_id"`
	From       *TelegramUser `json:"from"`
	Chat       *TelegramChat  `json:"chat"`
	Date       int64        `json:"date"`
	Text       string       `json:"text"`
}

// TelegramUser represents a Telegram user
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	UserName  string `json:"username"`
}

// TelegramChat represents a Telegram chat
type TelegramChat struct {
	ID int64 `json:"id"`
}

// isAllowed checks if user ID is in allowlist
func (b *TelegramBot) isAllowed(userID int64) bool {
	return b.allowedIDs[userID]
}

// convertToProtocolMessage converts Telegram message to protocol message
func (b *TelegramBot) convertToProtocolMessage(msg *TelegramMessage) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel":  "telegram",
			"text":     msg.Text,
			"user_id":  msg.From.ID,
			"username": msg.From.UserName,
		},
	}
}

// SendMessage sends a message to Telegram user
func (b *TelegramBot) SendMessage(ctx context.Context, chatID int64, text string) error {
	// Use Telegram Bot API
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.botToken)

	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}

// SendTypingIndicator sends typing indicator
func (b *TelegramBot) SendTypingIndicator(ctx context.Context, chatID int64) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", b.botToken)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"action":     "typing",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
