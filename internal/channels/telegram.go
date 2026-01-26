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
	botToken     string
	manager      *MessageManager
	userManager  *UserManager
	webhookURL   string
	webhookSecret string
	server       *http.Server
	stopChan     chan struct{}
	adminID      int64
	ctx          context.Context
}

// NewTelegramBot creates a new Telegram bot instance
func NewTelegramBot(token string, allowedUsers []int64, adminID int64) (*TelegramBot, error) {
	userManager := NewUserManager(allowedUsers)

	return &TelegramBot{
		botToken:     token,
		manager:      NewMessageManager(),
		userManager:  userManager,
		allowedIDs:    nil, // Use userManager instead
		webhookURL:     "",
		webhookSecret: "",
		server:        nil,
		stopChan:       make(chan struct{}),
		adminID:       adminID,
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
	if b.webhookURL != "" {
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
	// secret := r.URL.Query().Get("secret")

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
	if !b.userManager.Authorize(update.Message.From.ID) {
		log.Printf("🚫 Unauthorized access attempt: User ID %d", update.Message.From.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Handle commands
	if err := b.handleCommand(update.Message); err != nil {
		log.Printf("Error handling command: %v", err)
	}

	// Convert to protocol message and send to manager
	msg := b.convertToProtocolMessage(update.Message)

	if err := b.manager.Send(msg); err != nil {
		log.Printf("Error sending to message manager: %v", err)
	}
}

// handleCommand processes bot commands
func (b *TelegramBot) handleCommand(msg *TelegramMessage) error {
	if msg.Text == "" {
		return nil
	}

	// Check if it starts with /
	if msg.Text[0] != '/' {
		return nil
	}

	// Parse command
	parts := splitCommand(msg.Text)
	if len(parts) == 0 {
		return nil
	}

	command := parts[0]

	switch command {
	case "/adduser":
		if len(parts) != 2 {
			return fmt.Errorf("usage: /adduser <user_id>")
		}
		return b.userManager.AddUser(parseUserID(parts[1]))
	case "/removeuser":
		if len(parts) != 2 {
			return fmt.Errorf("usage: /removeuser <user_id>")
		}
		return b.userManager.RemoveUser(parseUserID(parts[1]))
	case "/listusers":
		users := b.userManager.GetAllowedUsers()
		response := fmt.Sprintf("✅ Authorized users:\n%s", formatUserList(users))
		_, err := b.SendMessage(b.ctx, msg.Chat.ID, response)
		return err
	case "/status":
		status := fmt.Sprintf("🤖 Bot Status:\n✅ Running\n👤 Admin: %d\n👥 Webhook: %s", b.adminID, b.webhookURL)
		_, err := b.SendMessage(b.ctx, msg.Chat.ID, status)
		return err
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

// splitCommand splits command into parts
func splitCommand(text string) []string {
	parts := make([]string, 0)
	current := ""
	inQuotes := false

	for _, ch := range text {
		if ch == ' ' {
			inQuotes = true
		} else if ch == '"' {
			inQuotes = true
		} else if (ch == ' ' || ch == '\t') && !inQuotes {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// parseUserID extracts user ID from command argument
func parseUserID(arg string) int64 {
	var id int64
	_, err := fmt.Sscanf(arg, "%d", &id)
	if err != nil {
		return 0
	}
	return id
}

// formatUserList formats user list for display
func formatUserList(users []int64) string {
	result := ""
	for _, id := range users {
		result += fmt.Sprintf("  - %d\n", id)
	}
	return result
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
		"chat_id": chatID,
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
