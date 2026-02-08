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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const (
	defaultTelegramWebhookPath             = "/telegram/webhook"
	defaultTelegramPollingTimeout          = 25 * time.Second
	defaultTelegramPollingLimit            = 100
	defaultTelegramPollingBackoffMin       = 1 * time.Second
	defaultTelegramPollingBackoffMax       = 30 * time.Second
	maxTelegramRequestBodyBytes      int64 = 512 * 1024
)

// TelegramBot implements Telegram channel.
type TelegramBot struct {
	botToken       string
	defaultAgent   string
	agentAllowlist AgentAllowlist

	handler     IncomingMessageHandler
	userManager *UserManager

	allowedChats           map[int64]struct{}
	allowedChatsConfigured bool

	adminID int64

	mode string

	webhookListenAddr      string
	webhookPath            string
	webhookPublicURL       string
	webhookSecretToken     string
	webhookRegisterOnStart bool
	webhookDeleteOnStop    bool
	webhookRegistered      bool

	pollingTimeout    time.Duration
	pollingLimit      int
	pollingOffsetFile string
	nextUpdateID      int64

	httpClient *http.Client
	sleeper    func(time.Duration)

	server    *http.Server
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time

	runningMu sync.RWMutex
	running   bool

	telemetryMu  sync.RWMutex
	lastActivity time.Time
	lastError    time.Time
}

// TelegramWebhookStatus exposes safe webhook lifecycle details for status output.
type TelegramWebhookStatus struct {
	RegisterOnStart      bool `json:"register_on_start,omitempty"`
	DeleteOnStop         bool `json:"delete_on_stop,omitempty"`
	PublicURLConfigured  bool `json:"public_url_configured,omitempty"`
	ListenAddrConfigured bool `json:"listen_addr_configured,omitempty"`
	Registered           bool `json:"registered,omitempty"`
}

// NewTelegramBot creates a new Telegram bot instance.
func NewTelegramBot(token string, allowedUsers []int64, adminID int64, defaultAgent string, allowedAgents []string) (*TelegramBot, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("telegram bot token is required")
	}

	userManager := NewUserManager(allowedUsers)
	if adminID != 0 {
		userManager.AddUser(adminID)
	}

	return &TelegramBot{
		botToken:       token,
		defaultAgent:   strings.TrimSpace(defaultAgent),
		agentAllowlist: NewAgentAllowlist(allowedAgents),
		userManager:    userManager,
		adminID:        adminID,
		webhookPath:    defaultTelegramWebhookPath,
		ctx:            context.Background(),
		startTime:      time.Now(),
		pollingTimeout: defaultTelegramPollingTimeout,
		pollingLimit:   defaultTelegramPollingLimit,
		httpClient:     &http.Client{Timeout: 35 * time.Second},
		sleeper:        time.Sleep,
	}, nil
}

// Name returns the bot name.
func (b *TelegramBot) Name() string {
	return "telegram"
}

// Mode returns the configured or active Telegram mode.
func (b *TelegramBot) Mode() string {
	return b.mode
}

// SetHandler sets the inbound message handler.
func (b *TelegramBot) SetHandler(handler IncomingMessageHandler) {
	b.handler = handler
}

// IsRunning reports whether the bot has been started.
func (b *TelegramBot) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

// WebhookStatus returns lifecycle configuration for webhook mode.
func (b *TelegramBot) WebhookStatus() TelegramWebhookStatus {
	return TelegramWebhookStatus{
		RegisterOnStart:      b.webhookRegisterOnStart,
		DeleteOnStop:         b.webhookDeleteOnStop,
		PublicURLConfigured:  b.webhookPublicURL != "",
		ListenAddrConfigured: b.webhookListenAddr != "",
		Registered:           b.webhookRegistered,
	}
}

// LastActivity reports the last time the bot saw a message or successfully sent one.
func (b *TelegramBot) LastActivity() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastActivity
}

// LastError reports the last time the bot encountered a channel error.
func (b *TelegramBot) LastError() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastError
}

func (b *TelegramBot) markActivity() {
	b.telemetryMu.Lock()
	b.lastActivity = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *TelegramBot) markError() {
	b.telemetryMu.Lock()
	b.lastError = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *TelegramBot) setRunning(running bool) {
	b.runningMu.Lock()
	b.running = running
	b.runningMu.Unlock()
}

// ConfigureMode sets the channel mode.
// Supported values: "polling", "webhook". Empty means auto.
func (b *TelegramBot) ConfigureMode(mode string) {
	b.mode = strings.ToLower(strings.TrimSpace(mode))
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

// ConfigureWebhookLifecycle controls webhook registration and cleanup behavior.
func (b *TelegramBot) ConfigureWebhookLifecycle(registerOnStart, deleteOnStop bool) {
	b.webhookRegisterOnStart = registerOnStart
	b.webhookDeleteOnStop = deleteOnStop
}

// ConfigurePolling configures long polling settings.
// timeoutSeconds defaults to 25 if <=0. limit defaults to 100 if <=0.
// offsetFile, if set, persists next update offset to avoid re-processing after restart.
func (b *TelegramBot) ConfigurePolling(timeoutSeconds int, limit int, offsetFile string) {
	if timeoutSeconds > 0 {
		b.pollingTimeout = time.Duration(timeoutSeconds) * time.Second
	} else {
		b.pollingTimeout = defaultTelegramPollingTimeout
	}

	if limit > 0 {
		b.pollingLimit = limit
	} else {
		b.pollingLimit = defaultTelegramPollingLimit
	}

	b.pollingOffsetFile = strings.TrimSpace(offsetFile)
}

// Start starts the Telegram bot.
func (b *TelegramBot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	log.Println("📱 Telegram bot starting...")

	mode := b.mode
	if mode == "" || mode == "auto" {
		if b.webhookListenAddr != "" || b.webhookPublicURL != "" {
			mode = "webhook"
		} else {
			mode = "polling"
		}
	}
	if mode != "polling" && mode != "webhook" {
		return fmt.Errorf("invalid telegram mode: %q (expected polling|webhook)", b.mode)
	}
	b.mode = mode

	if mode == "polling" {
		if err := b.deleteWebhook(b.ctx); err != nil {
			return err
		}
		b.webhookRegistered = false
		if err := b.loadPollingOffset(); err != nil {
			return err
		}
		b.startPollingLoop()
		log.Printf("🔁 Telegram polling enabled (timeout=%s, limit=%d, nextUpdateID=%d)", b.pollingTimeout, b.pollingLimit, b.nextUpdateID)
		b.setRunning(true)
		return nil
	}

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

	if b.webhookPublicURL != "" && b.webhookRegisterOnStart {
		if err := b.setWebhook(b.ctx); err != nil {
			return err
		}
		b.webhookRegistered = true
		log.Printf("🔗 Telegram webhook registered")
	}

	b.setRunning(true)
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

	if b.mode == "webhook" && b.webhookDeleteOnStop {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := b.deleteWebhook(ctx); err != nil {
			return err
		}
		b.webhookRegistered = false
	}

	b.setRunning(false)
	return nil
}

type telegramBoolResponse struct {
	OK          bool   `json:"ok"`
	Result      bool   `json:"result"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

func (b *TelegramBot) setWebhook(ctx context.Context) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", b.botToken)

	fail := func(err error) error {
		b.markError()
		return err
	}

	payload := map[string]interface{}{
		"url": b.webhookPublicURL,
	}
	if b.webhookSecretToken != "" {
		payload["secret_token"] = b.webhookSecretToken
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fail(fmt.Errorf("failed to marshal setWebhook payload: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return fail(fmt.Errorf("failed to create setWebhook request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fail(fmt.Errorf("failed to call setWebhook: %w", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("telegram setWebhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	var parsed telegramBoolResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fail(fmt.Errorf("failed to parse setWebhook response: %w", err))
	}
	if !parsed.OK || !parsed.Result {
		if parsed.Description != "" {
			return fail(fmt.Errorf("telegram setWebhook failed: %s", parsed.Description))
		}
		return fail(fmt.Errorf("telegram setWebhook failed"))
	}

	return nil
}

func (b *TelegramBot) deleteWebhook(ctx context.Context) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook", b.botToken)

	fail := func(err error) error {
		b.markError()
		return err
	}

	payload := map[string]interface{}{
		"drop_pending_updates": false,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fail(fmt.Errorf("failed to marshal deleteWebhook payload: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return fail(fmt.Errorf("failed to create deleteWebhook request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fail(fmt.Errorf("failed to call deleteWebhook: %w", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("telegram deleteWebhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	var parsed telegramBoolResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fail(fmt.Errorf("failed to parse deleteWebhook response: %w", err))
	}
	if !parsed.OK || !parsed.Result {
		if parsed.Description != "" {
			return fail(fmt.Errorf("telegram deleteWebhook failed: %s", parsed.Description))
		}
		return fail(fmt.Errorf("telegram deleteWebhook failed"))
	}

	return nil
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message"`
}

type telegramGetUpdatesResponse struct {
	OK          bool             `json:"ok"`
	Result      []telegramUpdate `json:"result"`
	ErrorCode   int              `json:"error_code"`
	Description string           `json:"description"`
}

func (b *TelegramBot) getUpdates(ctx context.Context) ([]telegramUpdate, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", b.botToken)

	payload := map[string]interface{}{
		"timeout":         int(b.pollingTimeout.Seconds()),
		"limit":           b.pollingLimit,
		"allowed_updates": []string{"message"},
	}
	if b.nextUpdateID > 0 {
		payload["offset"] = b.nextUpdateID
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal getUpdates payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create getUpdates request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call getUpdates: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram getUpdates returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed telegramGetUpdatesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse getUpdates response: %w", err)
	}
	if !parsed.OK {
		if parsed.Description != "" {
			return nil, fmt.Errorf("telegram getUpdates failed: %s", parsed.Description)
		}
		return nil, fmt.Errorf("telegram getUpdates failed")
	}

	return parsed.Result, nil
}

func (b *TelegramBot) startPollingLoop() {
	go func() {
		backoff := defaultTelegramPollingBackoffMin
		sleeper := b.sleeper
		if sleeper == nil {
			sleeper = time.Sleep
		}
		for {
			select {
			case <-b.ctx.Done():
				return
			default:
			}

			updates, err := b.getUpdates(b.ctx)
			if err != nil {
				b.markError()
				log.Printf("Telegram polling error: %v", err)
				sleeper(backoff)
				backoff = nextPollingBackoff(backoff)
				continue
			}
			backoff = defaultTelegramPollingBackoffMin

			for _, update := range updates {
				b.handleUpdate(update)
				b.nextUpdateID = update.UpdateID + 1
				if err := b.persistPollingOffset(); err != nil {
					log.Printf("Telegram polling offset persist error: %v", err)
				}
			}
		}
	}()
}

func nextPollingBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > defaultTelegramPollingBackoffMax {
		return defaultTelegramPollingBackoffMax
	}
	return next
}

func (b *TelegramBot) loadPollingOffset() error {
	if b.pollingOffsetFile == "" {
		return nil
	}

	data, err := os.ReadFile(b.pollingOffsetFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to read telegram polling offset file: %w", err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}

	next, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram polling offset value %q: %w", text, err)
	}
	if next < 0 {
		next = 0
	}
	b.nextUpdateID = next
	return nil
}

func (b *TelegramBot) persistPollingOffset() error {
	if b.pollingOffsetFile == "" {
		return nil
	}

	dir := filepath.Dir(b.pollingOffsetFile)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create offset dir: %w", err)
		}
	}

	tmp, err := os.CreateTemp(dir, ".telegram-offset-*")
	if err != nil {
		return fmt.Errorf("failed to create temp offset file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	if _, err := tmp.WriteString(strconv.FormatInt(b.nextUpdateID, 10) + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temp offset file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp offset file: %w", err)
	}

	if err := os.Rename(tmp.Name(), b.pollingOffsetFile); err != nil {
		return fmt.Errorf("failed to rename temp offset file: %w", err)
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

	body, err := io.ReadAll(io.LimitReader(r.Body, maxTelegramRequestBodyBytes))
	if err != nil {
		b.markError()
		log.Printf("Failed to read Telegram webhook body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	var update telegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		b.markError()
		log.Printf("Failed to parse Telegram webhook update: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	b.handleUpdate(update)
}

func (b *TelegramBot) handleUpdate(update telegramUpdate) {
	if update.Message == nil || update.Message.From == nil || update.Message.Chat == nil {
		return
	}
	b.markActivity()
	b.handleIncomingMessage(update.Message)
}

func (b *TelegramBot) handleIncomingMessage(message *TelegramMessage) {
	if message.Chat != nil && strings.TrimSpace(message.Chat.Type) != "" && message.Chat.Type != "private" {
		return
	}
	if b.allowedChatsConfigured {
		if _, ok := b.allowedChats[message.Chat.ID]; !ok {
			return
		}
	}
	if !b.userManager.Authorize(message.From.ID) {
		log.Printf("🚫 Unauthorized Telegram user: %d", message.From.ID)
		if isTelegramWhoamiCommand(message.Text) {
			reply := formatTelegramWhoamiReply(message, b.adminID)
			_ = b.SendMessage(b.ctx, message.Chat.ID, TruncateTelegramReply(reply))
			return
		}
		username := strings.TrimSpace(message.From.UserName)
		if username != "" {
			username = "@" + username
		} else {
			username = "(none)"
		}
		hint := fmt.Sprintf(
			"❌ Unauthorized. Ask an admin to add your Telegram user ID in channels.telegram.allowedUsers.\nUser ID: %d\nUsername: %s",
			message.From.ID,
			username,
		)
		_ = b.SendMessage(b.ctx, message.Chat.ID, TruncateTelegramReply(hint))
		return
	}

	if handled, cmdErr := b.handleCommand(message); handled {
		if cmdErr != nil {
			reply := fmt.Sprintf("❌ %v", cmdErr)
			if isAgentNotAllowedError(cmdErr) {
				reply = agentNotAllowedMessage(cmdErr, b.defaultAgent, b.agentAllowlist)
			} else if isAgentAllowlistError(cmdErr) {
				reply = fmt.Sprintf("%s\nTip: use /agents to see allowed agents.", reply)
			}
			_ = b.SendMessage(b.ctx, message.Chat.ID, TruncateTelegramReply(reply))
		}
		return
	}

	selection, err := ParseAgentSelection(message.Text)
	if err != nil {
		reply := fmt.Sprintf("❌ %v", err)
		if isAgentNotAllowedError(err) {
			reply = agentNotAllowedMessage(err, b.defaultAgent, b.agentAllowlist)
		} else if isAgentAllowlistError(err) {
			reply = fmt.Sprintf("%s\nTip: use /agents to see allowed agents.", reply)
		}
		_ = b.SendMessage(b.ctx, message.Chat.ID, TruncateTelegramReply(reply))
		return
	}

	if strings.TrimSpace(selection.Task) == "" {
		return
	}

	enforceSelection := selection.Specified || b.defaultAgent != "" || b.agentAllowlist.configured
	if enforceSelection && !isTelegramToolInvocation(selection.Task) {
		selection, err = ResolveAgentSelection(selection, b.defaultAgent, b.agentAllowlist)
		if err != nil {
			reply := fmt.Sprintf("❌ %v", err)
			if !selection.Specified && (isDefaultAgentMissingError(err) || isInvalidAgentNameError(err)) {
				reply = "❌ Default agent is missing or invalid.\nSet agents.ohMyCode.defaultAgent or use /agent <name> <task> (or /to <name> <task>).\nTip: use /agents to see allowed agents."
			} else if isAgentNotAllowedError(err) {
				reply = agentNotAllowedMessage(err, b.defaultAgent, b.agentAllowlist)
			} else if isAgentAllowlistError(err) {
				reply = fmt.Sprintf("%s\nTip: use /agents to see allowed agents.", reply)
			}
			_ = b.SendMessage(b.ctx, message.Chat.ID, TruncateTelegramReply(reply))
			return
		}
	}

	msg := b.convertToProtocolMessage(message, selection.Task, selection.Agent)

	if b.handler != nil {
		replyText, err := b.handler.HandleIncoming(b.ctx, msg)
		if err != nil {
			log.Printf("Telegram handler error: %v", err)
			replyText = "❌ Something went wrong. Please try again."
		}
		if strings.TrimSpace(replyText) != "" {
			_ = b.SendMessage(b.ctx, message.Chat.ID, TruncateTelegramReply(replyText))
		}
		return
	}

	if selection.Task != "" {
		reply := fmt.Sprintf("echo: %s", selection.Task)
		_ = b.SendMessage(b.ctx, message.Chat.ID, reply)
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
	command = strings.ToLower(command)
	if command == "/agent" || command == "/to" || isRuntimeToolCommand(command) {
		return false, nil
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
	case "/help", "/start":
		return true, b.SendMessage(b.ctx, msg.Chat.ID, b.helpText())

	case "/ping":
		return true, b.SendMessage(b.ctx, msg.Chat.ID, "pong")

	case "/whoami":
		reply := formatTelegramWhoamiReply(msg, b.adminID)
		return true, b.SendMessage(b.ctx, msg.Chat.ID, TruncateTelegramReply(reply))

	case "/agents":
		names := b.agentAllowlist.Names()
		defaultName := strings.TrimSpace(b.defaultAgent)
		if len(names) > 0 && defaultName != "" {
			names = filterOutAgentName(names, defaultName)
		}
		if len(names) == 0 && defaultName == "" {
			return true, b.SendMessage(b.ctx, msg.Chat.ID, noAgentsConfiguredMessage)
		}
		var sb strings.Builder
		sb.WriteString("Allowed agents:\n")
		if defaultName != "" {
			sb.WriteString(fmt.Sprintf("Default agent: %s\n", defaultName))
		}
		for _, name := range names {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		return true, b.SendMessage(b.ctx, msg.Chat.ID, strings.TrimSpace(sb.String()))

	case "/monitor":
		agentName, lines, err := parseMonitorArgs(parts)
		if err != nil {
			return true, err
		}
		if err := validateAgentCommandName(agentName, b.defaultAgent, b.agentAllowlist); err != nil {
			return true, err
		}
		lifecycle, ok := b.handler.(AgentLifecycle)
		if !ok || lifecycle == nil {
			return true, errors.New("agent-manager is not available")
		}
		out, err := lifecycle.MonitorAgent(b.ctx, agentName, lines)
		if err != nil {
			return true, b.sanitizeLifecycleError(command, err)
		}
		if strings.TrimSpace(out) == "" {
			out = "No output from agent-monitor."
		}
		return true, b.SendMessage(b.ctx, msg.Chat.ID, TruncateTelegramReply(out))

	case "/startagent":
		if err := requireAdmin(); err != nil {
			return true, err
		}
		if len(parts) != 2 {
			return true, fmt.Errorf("usage: /startagent <name>")
		}
		agentName := strings.TrimSpace(parts[1])
		if err := validateAgentCommandName(agentName, b.defaultAgent, b.agentAllowlist); err != nil {
			return true, err
		}
		lifecycle, ok := b.handler.(AgentLifecycle)
		if !ok || lifecycle == nil {
			return true, errors.New("agent-manager is not available")
		}
		out, err := lifecycle.StartAgent(b.ctx, agentName)
		if err != nil {
			return true, b.sanitizeLifecycleError(command, err)
		}
		if strings.TrimSpace(out) == "" {
			out = fmt.Sprintf("✅ Started agent %s", agentName)
		}
		return true, b.SendMessage(b.ctx, msg.Chat.ID, TruncateTelegramReply(out))

	case "/stopagent":
		if err := requireAdmin(); err != nil {
			return true, err
		}
		if len(parts) != 2 {
			return true, fmt.Errorf("usage: /stopagent <name>")
		}
		agentName := strings.TrimSpace(parts[1])
		if err := validateAgentCommandName(agentName, b.defaultAgent, b.agentAllowlist); err != nil {
			return true, err
		}
		lifecycle, ok := b.handler.(AgentLifecycle)
		if !ok || lifecycle == nil {
			return true, errors.New("agent-manager is not available")
		}
		out, err := lifecycle.StopAgent(b.ctx, agentName)
		if err != nil {
			return true, b.sanitizeLifecycleError(command, err)
		}
		if strings.TrimSpace(out) == "" {
			out = fmt.Sprintf("✅ Stopped agent %s", agentName)
		}
		return true, b.SendMessage(b.ctx, msg.Chat.ID, TruncateTelegramReply(out))

	case "/doctor":
		if err := requireAdmin(); err != nil {
			return true, err
		}
		lifecycle, ok := b.handler.(AgentLifecycle)
		if !ok || lifecycle == nil {
			return true, errors.New("agent-manager is not available")
		}
		out, err := lifecycle.Doctor(b.ctx)
		if err != nil {
			return true, b.sanitizeLifecycleError(command, err)
		}
		if strings.TrimSpace(out) == "" {
			out = "✅ agent-manager doctor completed"
		}
		return true, b.SendMessage(b.ctx, msg.Chat.ID, TruncateTelegramReply(out))

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
		status := fmt.Sprintf(
			"🤖 Bot Status:\n✅ Running\n👤 Admin: %d\n🔧 Mode: %s\n🌐 Webhook listen: %s%s\n🔗 Webhook public: %s\n🔁 Polling timeout: %s\n🔁 Polling limit: %d\n🧾 Polling offset file: %s\n⏱ Uptime: %s",
			b.adminID,
			b.mode,
			b.webhookListenAddr,
			b.webhookPath,
			b.webhookPublicURL,
			b.pollingTimeout,
			b.pollingLimit,
			b.pollingOffsetFile,
			uptime,
		)
		return true, b.SendMessage(b.ctx, msg.Chat.ID, status)

	default:
		return true, fmt.Errorf("unknown command: %s", command)
	}
}

func (b *TelegramBot) sanitizeLifecycleError(command string, err error) error {
	log.Printf("Telegram command %s failed: %v", command, err)
	return errors.New("agent-manager error; please check server logs")
}

func isRuntimeToolCommand(command string) bool {
	return command == "/tool" ||
		command == "/tools" ||
		strings.HasPrefix(command, "/tool:") ||
		strings.HasPrefix(command, "/tools:")
}

func isAgentAllowlistError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "agent") && strings.Contains(msg, "not allowed") ||
		strings.Contains(msg, "invalid agent name")
}

func isDefaultAgentMissingError(err error) bool {
	return errors.Is(err, errDefaultAgentMissing)
}

func isInvalidAgentNameError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "invalid agent name")
}

func isAgentNotAllowedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not allowed")
}

func (b *TelegramBot) helpText() string {
	var sb strings.Builder
	sb.WriteString("FractalBot Telegram Help\n")
	sb.WriteString("\n")
	sb.WriteString("Commands:\n")
	sb.WriteString("  /help - show this help\n")
	sb.WriteString("  /ping - simple health check\n")
	sb.WriteString("  /whoami - show your Telegram IDs\n")
	sb.WriteString("  /status - bot status\n")
	sb.WriteString("  /agents - list allowed agents\n")
	sb.WriteString("  /monitor <name> [lines] - show recent agent output\n")
	sb.WriteString("  /adduser <user_id> - admin only\n")
	sb.WriteString("  /removeuser <user_id> - admin only\n")
	sb.WriteString("  /listusers - admin only\n")
	sb.WriteString("  /startagent <name> - admin only\n")
	sb.WriteString("  /stopagent <name> - admin only\n")
	sb.WriteString("  /doctor - admin only\n")
	sb.WriteString("\n")
	sb.WriteString("Runtime tools (if enabled):\n")
	sb.WriteString("  /tools - list enabled tools (same as /tool tools.list)\n")
	sb.WriteString("  /tools@BotName - same\n")
	sb.WriteString("  /tool <name> <args...> (try: /tool tools.list)\n")
	sb.WriteString("  /tool@BotName <name> <args...>\n")
	sb.WriteString("\n")
	sb.WriteString("Agent routing:\n")
	sb.WriteString("  /agent <name> <task...>\n")
	sb.WriteString("  /to <name> <task...> (alias of /agent)\n")
	sb.WriteString("  /agents - see available agents\n")
	sb.WriteString("  Note: if an allowlist is configured, only allowlisted agents can be used.\n")
	if b.defaultAgent != "" {
		sb.WriteString(fmt.Sprintf("Default agent: %s\n", b.defaultAgent))
	}
	if names := b.agentAllowlist.Names(); len(names) > 0 {
		sb.WriteString("Allowed agents:\n")
		for _, name := range names {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}
	return strings.TrimSpace(sb.String())
}

func formatTelegramWhoamiReply(msg *TelegramMessage, adminID int64) string {
	username := strings.TrimSpace(msg.From.UserName)
	if username != "" {
		username = "@" + username
	} else {
		username = "(none)"
	}
	isAdmin := adminID != 0 && msg.From.ID == adminID
	return fmt.Sprintf(
		"User ID: %d\nUsername: %s\nChat ID: %d\nIs admin: %t",
		msg.From.ID,
		username,
		msg.Chat.ID,
		isAdmin,
	)
}

func (b *TelegramBot) setAllowedChats(chatIDs []int64) {
	if b == nil {
		return
	}
	allowed := make(map[int64]struct{})
	for _, id := range chatIDs {
		if id == 0 {
			continue
		}
		allowed[id] = struct{}{}
	}
	if len(allowed) == 0 {
		b.allowedChats = nil
		b.allowedChatsConfigured = false
		return
	}
	b.allowedChats = allowed
	b.allowedChatsConfigured = true
}

func isTelegramToolInvocation(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return hasTelegramToolPrefix(lower, "/tools") ||
		hasTelegramToolPrefix(lower, "/tool") ||
		hasTelegramToolPrefix(lower, "tool")
}

func isTelegramWhoamiCommand(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return false
	}
	parts := splitCommand(trimmed)
	if len(parts) == 0 {
		return false
	}
	command := parts[0]
	if idx := strings.IndexByte(command, '@'); idx != -1 {
		command = command[:idx]
	}
	return strings.ToLower(command) == "/whoami"
}

func hasTelegramToolPrefix(text, prefix string) bool {
	if !strings.HasPrefix(text, prefix) {
		return false
	}
	if len(text) == len(prefix) {
		return true
	}
	switch text[len(prefix)] {
	case ' ', ':', '@':
		return true
	default:
		return false
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
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

func (b *TelegramBot) convertToProtocolMessage(msg *TelegramMessage, text, agent string) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel":  "telegram",
			"text":     text,
			"agent":    agent,
			"chat_id":  msg.Chat.ID,
			"chatType": msg.Chat.Type,
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
		b.markError()
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		b.markError()
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.markError()
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		b.markError()
		return fmt.Errorf("telegram API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	b.markActivity()
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
		b.markError()
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonPayload))
	if err != nil {
		b.markError()
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		b.markError()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.markError()
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	b.markActivity()
	return nil
}
