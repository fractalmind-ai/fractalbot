package channels

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const defaultTelegramWebhookPath = "/telegram/webhook"

// TelegramBot implements Telegram channel.
type TelegramBot struct {
	botToken string

	manager     *MessageManager
	userManager *UserManager

	adminID int64

	webhookListenAddr  string
	webhookPath        string
	webhookPublicURL   string
	webhookSecretToken string

	server    *http.Server
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time
}

// NewTelegramBot creates a new Telegram bot instance.
func NewTelegramBot(token string, allowedUsers []int64, adminID int64) (*TelegramBot, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("telegram bot token is required")
	}

	userManager := NewUserManager(allowedUsers)
	if adminID != 0 {
		userManager.AddUser(adminID)
	}

	return &TelegramBot{
		botToken:    token,
		manager:     NewMessageManager(),
		userManager: userManager,
		adminID:     adminID,
		webhookPath: defaultTelegramWebhookPath,
		ctx:         context.Background(),
		startTime:   time.Now(),
	}, nil
}

// Name returns the bot name.
func (b *TelegramBot) Name() string {
	return "telegram"
}

// ConfigureWebhook configures webhook settings.
// listenAddr is the local server bind address (e.g. "0.0.0.0:18790").
// publicURL is the externally reachable HTTPS URL for Telegram to call.
func (b *TelegramBot) ConfigureWebhook(listenAddr, path, publicURL, secretToken string) {
	b.webhookListenAddr = strings.TrimSpace(listenAddr)
	if strings.TrimSpace(path) != "" {
		b.webhookPath = strings.TrimSpace(path)
	}
	b.webhookPublicURL = strings.TrimSpace(publicURL)
	b.webhookSecretToken = strings.TrimSpace(secretToken)
}

// Start starts the Telegram bot.
func (b *TelegramBot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	log.Println("📱 Telegram bot starting...")

	if b.webhookListenAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc(b.webhookPath, b.handleWebhook)
		mux.HandleFunc("/telegram/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		})

		b.server = &http.Server{
			Addr:              b.webhookListenAddr,
			Handler:           mux,
			ErrorLog:          log.New(os.Stderr, "telegram-webhook: ", log.LstdFlags),
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		go func() {
			log.Printf("📡 Telegram webhook server listening on %s%s", b.webhookListenAddr, b.webhookPath)
			if err := b.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Telegram webhook server error: %v", err)
			}
		}()
	}

	if b.webhookPublicURL != "" {
		if err := b.setWebhook(b.ctx); err != nil {
			return err
		}
		log.Printf("🔗 Telegram webhook registered: %s", b.webhookPublicURL)
	}

	return nil
}

// Stop gracefully shuts down the bot.
func (b *TelegramBot) Stop() error {
	log.Println("🛑 Stopping Telegram bot...")

	if b.cancel != nil {
		b.cancel()
	}

	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.server.Shutdown(ctx)
	}

	return nil
}

type telegramSetWebhookResponse struct {
	OK          bool   `json:"ok"`
	Result      bool   `json:"result"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

func (b *TelegramBot) setWebhook(ctx context.Context) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", b.botToken)

	payload := map[string]interface{}{
		"url": b.webhookPublicURL,
	}
	if b.webhookSecretToken != "" {
		payload["secret_token"] = b.webhookSecretToken
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal setWebhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create setWebhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call setWebhook: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram setWebhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed telegramSetWebhookResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("failed to parse setWebhook response: %w", err)
	}
	if !parsed.OK || !parsed.Result {
		if parsed.Description != "" {
			return fmt.Errorf("telegram setWebhook failed: %s", parsed.Description)
		}
		return fmt.Errorf("telegram setWebhook failed")
	}

	return nil
}

// handleWebhook handles incoming Telegram webhook updates.
func (b *TelegramBot) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if b.webhookSecretToken != "" {
		got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(b.webhookSecretToken)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 512*1024))
	if err != nil {
		log.Printf("Failed to read Telegram webhook body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	var update struct {
		UpdateID int64            `json:"update_id"`
		Message  *TelegramMessage `json:"message"`
	}

	if err := json.Unmarshal(body, &update); err != nil {
		log.Printf("Failed to parse Telegram webhook update: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if update.Message == nil || update.Message.From == nil || update.Message.Chat == nil {
		return
	}

	if !b.userManager.Authorize(update.Message.From.ID) {
		log.Printf("🚫 Unauthorized Telegram user: %d", update.Message.From.ID)
		return
	}

	if handled, cmdErr := b.handleCommand(update.Message); handled {
		if cmdErr != nil {
			_ = b.SendMessage(b.ctx, update.Message.Chat.ID, fmt.Sprintf("❌ %v", cmdErr))
		}
		return
	}

	if update.Message.Text != "" {
		reply := fmt.Sprintf("echo: %s", update.Message.Text)
		_ = b.SendMessage(b.ctx, update.Message.Chat.ID, reply)
	}

	msg := b.convertToProtocolMessage(update.Message)
	if err := b.manager.Send(msg); err != nil {
		log.Printf("Error routing Telegram message: %v", err)
	}
}

func (b *TelegramBot) handleCommand(msg *TelegramMessage) (bool, error) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return false, nil
	}
	if !strings.HasPrefix(text, "/") {
		return false, nil
	}

	parts := splitCommand(text)
	if len(parts) == 0 {
		return true, nil
	}

	command := parts[0]
	if idx := strings.IndexByte(command, '@'); idx != -1 {
		command = command[:idx]
	}

	requireAdmin := func() error {
		if b.adminID == 0 {
			return errors.New("adminID not configured")
		}
		if msg.From.ID != b.adminID {
			return fmt.Errorf("unauthorized: admin only")
		}
		return nil
	}

	switch command {
	case "/adduser":
		if err := requireAdmin(); err != nil {
			return true, err
		}
		if len(parts) != 2 {
			return true, fmt.Errorf("usage: /adduser <user_id>")
		}
		userID := parseUserID(parts[1])
		if userID == 0 {
			return true, fmt.Errorf("invalid user id")
		}
		b.userManager.AddUser(userID)
		return true, b.SendMessage(b.ctx, msg.Chat.ID, fmt.Sprintf("✅ Added user %d", userID))

	case "/removeuser":
		if err := requireAdmin(); err != nil {
			return true, err
		}
		if len(parts) != 2 {
			return true, fmt.Errorf("usage: /removeuser <user_id>")
		}
		userID := parseUserID(parts[1])
		if userID == 0 {
			return true, fmt.Errorf("invalid user id")
		}
		b.userManager.RemoveUser(userID)
		return true, b.SendMessage(b.ctx, msg.Chat.ID, fmt.Sprintf("✅ Removed user %d", userID))

	case "/listusers":
		if err := requireAdmin(); err != nil {
			return true, err
		}
		users := b.userManager.GetAllowedUsers()
		response := fmt.Sprintf("✅ Authorized users:\n%s", formatUserList(users))
		return true, b.SendMessage(b.ctx, msg.Chat.ID, response)

	case "/status":
		uptime := time.Since(b.startTime).Truncate(time.Second)
		status := fmt.Sprintf("🤖 Bot Status:\n✅ Running\n👤 Admin: %d\n🌐 Webhook listen: %s%s\n🔗 Webhook public: %s\n⏱ Uptime: %s",
			b.adminID,
			b.webhookListenAddr,
			b.webhookPath,
			b.webhookPublicURL,
			uptime,
		)
		return true, b.SendMessage(b.ctx, msg.Chat.ID, status)

	default:
		return true, fmt.Errorf("unknown command: %s", command)
	}
}

func splitCommand(text string) []string {
	return strings.Fields(text)
}

func parseUserID(arg string) int64 {
	var id int64
	_, err := fmt.Sscanf(arg, "%d", &id)
	if err != nil {
		return 0
	}
	return id
}

func formatUserList(users []int64) string {
	var sb strings.Builder
	for _, id := range users {
		sb.WriteString(fmt.Sprintf("  - %d\n", id))
	}
	return sb.String()
}

// TelegramMessage represents a Telegram message.
type TelegramMessage struct {
	MessageID int64         `json:"message_id"`
	From      *TelegramUser `json:"from"`
	Chat      *TelegramChat `json:"chat"`
	Date      int64         `json:"date"`
	Text      string        `json:"text"`
}

// TelegramUser represents a Telegram user.
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	UserName  string `json:"username"`
}

// TelegramChat represents a Telegram chat.
type TelegramChat struct {
	ID int64 `json:"id"`
}

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

// SendMessage sends a message to Telegram.
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return fmt.Errorf("telegram API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

// SendTypingIndicator sends typing indicator.
func (b *TelegramBot) SendTypingIndicator(ctx context.Context, chatID int64) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", b.botToken)

	payload := map[string]interface{}{
		"chat_id": chatID,
		"action":  "typing",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
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
