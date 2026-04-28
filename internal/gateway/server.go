package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/agent"
	"github.com/fractalmind-ai/fractalbot/internal/bus"
	"github.com/fractalmind-ai/fractalbot/internal/channels"
	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/gorilla/websocket"
)

// Server represents the gateway WebSocket server
type Server struct {
	config       *config.Config
	upgrader     websocket.Upgrader
	clients      map[string]*Client
	clientsMutex sync.RWMutex
	httpServer   *http.Server
	agentManager *agent.Manager
	messageBus   *bus.MessageBus
	startTime    time.Time
}

// NewServer creates a new gateway server
func NewServer(cfg *config.Config) (*Server, error) {
	if cfg.Gateway == nil {
		return nil, fmt.Errorf("gateway config is required")
	}

	// Initialize channels
	channelManager := channels.NewManager(cfg.Channels, cfg.Agents)

	// Initialize agent manager
	agentManager := agent.NewManager(cfg.Agents)
	agentManager.ChannelManager = channelManager

	// Initialize message bus — decouples channels from agent router
	messageBus := bus.New(agentManager, channelManager, 64, 64)
	messageBus.Start()

	// Wire bus as the inbound handler (bus implements IncomingMessageHandler)
	channelManager.SetHandler(messageBus)

	return &Server{
		config: cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     buildOriginChecker(cfg.Gateway.AllowedOrigins),
		},
		clients:      make(map[string]*Client),
		agentManager: agentManager,
		messageBus:   messageBus,
	}, nil
}

// Start starts the gateway server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Status endpoint
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/api/v1/message/send", s.handleMessageSend)

	if s.startTime.IsZero() {
		s.startTime = time.Now()
	}

	// Start HTTP server
	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", s.config.Gateway.Bind, s.config.Gateway.Port),
		Handler:           mux,
		ErrorLog:          log.New(os.Stderr, "HTTP: ", log.LstdFlags),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Start channels
	if s.agentManager.ChannelManager != nil {
		if err := s.agentManager.ChannelManager.Start(ctx); err != nil {
			return fmt.Errorf("failed to start channels: %w", err)
		}
	}

	// Start agent manager
	if err := s.agentManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent manager: %w", err)
	}

	go func() {
		log.Printf("🌐 HTTP server listening on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close bus first — drain pending messages while channels still run
	if s.messageBus != nil {
		s.messageBus.Close()
		s.messageBus.Wait()
	}

	if s.agentManager != nil {
		if s.agentManager.ChannelManager != nil {
			if err := s.agentManager.ChannelManager.Stop(); err != nil {
				return fmt.Errorf("failed to stop channels: %w", err)
			}
		}
		if err := s.agentManager.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop agent manager: %w", err)
		}
	}

	// Disconnect all clients
	clients := s.snapshotClients()
	for _, client := range clients {
		client.Close()
	}

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
	}

	return nil
}

func buildOriginChecker(allowed []string) func(*http.Request) bool {
	configured := len(allowed) > 0
	allowedSet := make(map[string]struct{})
	for _, origin := range allowed {
		normalized, ok := normalizeOrigin(origin)
		if !ok {
			continue
		}
		allowedSet[normalized] = struct{}{}
	}

	return func(r *http.Request) bool {
		if !configured {
			return true
		}
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			return false
		}
		normalized, ok := normalizeOrigin(origin)
		if !ok {
			return false
		}
		_, ok = allowedSet[normalized]
		return ok
	}
}

func normalizeOrigin(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	return fmt.Sprintf("%s://%s", strings.ToLower(parsed.Scheme), strings.ToLower(parsed.Host)), true
}

// handleWebSocket handles incoming WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	conn.SetReadLimit(readLimit)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	clientID := r.URL.Query().Get("session")
	if clientID == "" {
		clientID = generateClientID()
	}

	client := NewClient(clientID, conn, s)

	s.clientsMutex.Lock()
	s.clients[clientID] = client
	s.clientsMutex.Unlock()

	log.Printf("🔌 Client connected: %s", clientID)

	// Handle client messages
	go client.Handle()
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// GetAgentManager returns the agent manager
func (s *Server) GetAgentManager() *agent.Manager {
	return s.agentManager
}

type healthResponse struct {
	Status            string               `json:"status"`
	Uptime            string               `json:"uptime"`
	Channels          []healthChannelEntry `json:"channels"`
	MessagesProcessed int64                `json:"messages_processed"`
}

type healthChannelEntry struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Duration(0)
	if !s.startTime.IsZero() {
		uptime = time.Since(s.startTime)
	}

	var messagesProcessed int64
	if s.messageBus != nil {
		stats := s.messageBus.Stats()
		messagesProcessed = stats.InboundProcessed + stats.OutboundProcessed
	}

	chEntries := make([]healthChannelEntry, 0)
	if s.agentManager != nil && s.agentManager.ChannelManager != nil {
		for _, ch := range s.agentManager.ChannelManager.List() {
			chEntries = append(chEntries, healthChannelEntry{
				Name:    ch.Name(),
				Running: ch.IsRunning(),
			})
		}
	}

	writeJSON(w, http.StatusOK, healthResponse{
		Status:            "ok",
		Uptime:            uptime.String(),
		Channels:          chEntries,
		MessagesProcessed: messagesProcessed,
	})
}

type statusResponse struct {
	Status        string          `json:"status"`
	ActiveClients int             `json:"active_clients"`
	Uptime        string          `json:"uptime"`
	Channels      []channelStatus `json:"channels,omitempty"`
	Agents        *agentStatus    `json:"agents,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Duration(0)
	if !s.startTime.IsZero() {
		uptime = time.Since(s.startTime)
	}

	resp := statusResponse{
		Status:        "ok",
		ActiveClients: s.activeClients(),
		Uptime:        uptime.String(),
		Channels:      s.channelStatus(),
		Agents:        s.agentStatus(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type messageSendRequest struct {
	Channel  string `json:"channel"`
	To       string `json:"to"`
	Text     string `json:"text"`
	ThreadTS string `json:"thread_ts,omitempty"`
}

type messageSendResponse struct {
	Status      string `json:"status"`
	Channel     string `json:"channel,omitempty"`
	To          string `json:"to,omitempty"`
	ThreadTS    string `json:"thread_ts,omitempty"`
	ChannelID   string `json:"channel_id,omitempty"`
	ChannelName string `json:"channel_name,omitempty"`
	MessageTS   string `json:"message_ts,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (s *Server) handleMessageSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, messageSendResponse{Status: "error", Error: "method not allowed"})
		return
	}

	var request messageSendRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, messageSendResponse{Status: "error", Error: "invalid JSON payload"})
		return
	}

	request.Channel = strings.ToLower(strings.TrimSpace(request.Channel))
	request.To = strings.TrimSpace(request.To)
	request.Text = strings.TrimSpace(request.Text)
	request.ThreadTS = strings.TrimSpace(request.ThreadTS)

	if request.Channel == "" {
		writeJSON(w, http.StatusBadRequest, messageSendResponse{Status: "error", Error: "channel is required"})
		return
	}
	if request.To == "" {
		writeJSON(w, http.StatusBadRequest, messageSendResponse{Status: "error", Error: "to is required"})
		return
	}
	if request.Text == "" {
		writeJSON(w, http.StatusBadRequest, messageSendResponse{Status: "error", Error: "text is required"})
		return
	}

	if s.messageBus == nil {
		writeJSON(w, http.StatusServiceUnavailable, messageSendResponse{Status: "error", Error: "message bus unavailable"})
		return
	}

	result, err := s.messageBus.PublishOutbound(r.Context(), request.Channel, channels.OutboundMessage{
		To:       request.To,
		Text:     request.Text,
		ThreadTS: request.ThreadTS,
	})
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, messageSendResponse{Status: "error", Error: err.Error()})
		return
	}

	resp := messageSendResponse{
		Status:   "ok",
		Channel:  request.Channel,
		To:       request.To,
		ThreadTS: request.ThreadTS,
	}
	if result != nil {
		resp.ChannelID = result.ChannelID
		resp.ChannelName = result.ChannelName
		resp.MessageTS = result.MessageTS
		if result.ThreadTS != "" {
			resp.ThreadTS = result.ThreadTS
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to encode JSON response (status=%s): %v", strconv.Itoa(statusCode), err)
	}
}

func (s *Server) activeClients() int {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	return len(s.clients)
}

type channelStatus struct {
	Name         string                          `json:"name"`
	Enabled      bool                            `json:"enabled"`
	Running      bool                            `json:"running"`
	Mode         string                          `json:"mode,omitempty"`
	Provider     string                          `json:"provider,omitempty"`
	Webhook      *channels.TelegramWebhookStatus `json:"webhook,omitempty"`
	Callback     *callbackStatus                 `json:"callback,omitempty"`
	Polling      *pollingStatus                  `json:"polling,omitempty"`
	LastError    string                          `json:"last_error"`
	LastActivity string                          `json:"last_activity"`
}

type callbackStatus struct {
	ListenAddrConfigured bool `json:"listen_addr_configured"`
	PathConfigured       bool `json:"path_configured"`
	TokenConfigured      bool `json:"token_configured"`
	AESKeyConfigured     bool `json:"aes_key_configured"`
}

type pollingStatus struct {
	BaseURLConfigured   bool   `json:"base_url_configured"`
	TokenConfigured     bool   `json:"token_configured"`
	StateFileConfigured bool   `json:"state_file_configured"`
	IntervalSeconds     int    `json:"interval_seconds,omitempty"`
	CursorPresent       bool   `json:"cursor_present,omitempty"`
	CursorPreview       string `json:"cursor_preview,omitempty"`
	StateFileExists     bool   `json:"state_file_exists,omitempty"`
	LastPollAt          string `json:"last_poll_at,omitempty"`
	LastPollMessages    int    `json:"last_poll_messages,omitempty"`
}

type agentStatus struct {
	WorkspaceConfigured bool            `json:"workspace_configured"`
	MaxConcurrent       int             `json:"max_concurrent,omitempty"`
	OhMyCode            *ohMyCodeStatus `json:"oh_my_code,omitempty"`
}

type ohMyCodeRoutingStatus struct {
	SelectedAgent string `json:"selected_agent,omitempty"`
	Channel       string `json:"channel,omitempty"`
	ChatID        string `json:"chat_id,omitempty"`
	UserID        string `json:"user_id,omitempty"`
	Username      string `json:"username,omitempty"`
	Status        string `json:"status,omitempty"`
	Error         string `json:"error,omitempty"`
	RecordedAt    string `json:"recorded_at,omitempty"`
}

type ohMyCodeStatus struct {
	Enabled             bool                   `json:"enabled"`
	WorkspaceConfigured bool                   `json:"workspace_configured"`
	DefaultAgent        string                 `json:"default_agent,omitempty"`
	AllowedAgents       []string               `json:"allowed_agents,omitempty"`
	LastRouting         *ohMyCodeRoutingStatus `json:"last_routing,omitempty"`
}

func (s *Server) channelStatus() []channelStatus {
	if s.config == nil || s.config.Channels == nil {
		return nil
	}

	statuses := make([]channelStatus, 0, 5)
	getChannel := func(name string) channels.Channel {
		if s.agentManager == nil || s.agentManager.ChannelManager == nil {
			return nil
		}
		return s.agentManager.ChannelManager.Get(name)
	}
	isRunning := func(ch channels.Channel) bool {
		if ch == nil {
			return false
		}
		return ch.IsRunning()
	}
	telemetry := func(ch channels.Channel) (string, string) {
		if ch == nil {
			return "", ""
		}
		if provider, ok := ch.(channels.TelemetryProvider); ok {
			return formatStatusTime(provider.LastError()), formatStatusTime(provider.LastActivity())
		}
		return "", ""
	}

	if s.config.Channels.WeChat != nil {
		ch := getChannel("wechat")
		lastError, lastActivity := telemetry(ch)
		polling := wechatPollingStatusFromConfig(s.config.Channels.WeChat)
		if bot, ok := ch.(*channels.WeChatBot); ok && polling != nil {
			runtime := bot.PollingRuntimeStatus()
			polling.CursorPresent = runtime.CursorPresent
			polling.CursorPreview = runtime.CursorPreview
			polling.StateFileExists = runtime.StateFileExists
			polling.LastPollAt = formatStatusTime(runtime.LastPollAt)
			polling.LastPollMessages = runtime.LastPollMsgs
		}
		statuses = append(statuses, channelStatus{
			Name:         "wechat",
			Enabled:      s.config.Channels.WeChat.Enabled,
			Running:      isRunning(ch),
			Mode:         wechatModeFromConfig(s.config.Channels.WeChat),
			Provider:     wechatProviderFromConfig(s.config.Channels.WeChat),
			Callback:     wechatCallbackStatusFromConfig(s.config.Channels.WeChat),
			Polling:      polling,
			LastError:    lastError,
			LastActivity: lastActivity,
		})
	}

	if s.config.Channels.Telegram != nil {
		mode := telegramModeFromConfig(s.config.Channels.Telegram)
		webhookStatus := telegramWebhookStatusFromConfig(s.config.Channels.Telegram)
		ch := getChannel("telegram")
		lastError, lastActivity := telemetry(ch)
		if bot, ok := ch.(*channels.TelegramBot); ok {
			if bot.Mode() != "" {
				mode = bot.Mode()
			}
			status := bot.WebhookStatus()
			webhookStatus = &status
		}
		statuses = append(statuses, channelStatus{
			Name:         "telegram",
			Enabled:      s.config.Channels.Telegram.Enabled,
			Running:      isRunning(ch),
			Mode:         mode,
			Webhook:      webhookStatus,
			LastError:    lastError,
			LastActivity: lastActivity,
		})
	}
	if s.config.Channels.Slack != nil {
		ch := getChannel("slack")
		lastError, lastActivity := telemetry(ch)
		statuses = append(statuses, channelStatus{
			Name:         "slack",
			Enabled:      s.config.Channels.Slack.Enabled,
			Running:      isRunning(ch),
			LastError:    lastError,
			LastActivity: lastActivity,
		})
	}
	if s.config.Channels.Feishu != nil {
		ch := getChannel("feishu")
		lastError, lastActivity := telemetry(ch)
		statuses = append(statuses, channelStatus{
			Name:         "feishu",
			Enabled:      s.config.Channels.Feishu.Enabled,
			Running:      isRunning(ch),
			LastError:    lastError,
			LastActivity: lastActivity,
		})
	}
	if s.config.Channels.Discord != nil {
		ch := getChannel("discord")
		lastError, lastActivity := telemetry(ch)
		statuses = append(statuses, channelStatus{
			Name:         "discord",
			Enabled:      s.config.Channels.Discord.Enabled,
			Running:      isRunning(ch),
			LastError:    lastError,
			LastActivity: lastActivity,
		})
	}
	if s.config.Channels.IMessage != nil {
		ch := getChannel("imessage")
		lastError, lastActivity := telemetry(ch)
		statuses = append(statuses, channelStatus{
			Name:         "imessage",
			Enabled:      s.config.Channels.IMessage.Enabled,
			Running:      isRunning(ch),
			LastError:    lastError,
			LastActivity: lastActivity,
		})
	}

	return statuses
}

func telegramModeFromConfig(cfg *config.TelegramConfig) string {
	if cfg == nil {
		return ""
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" || mode == "auto" {
		if strings.TrimSpace(cfg.WebhookListenAddr) != "" || strings.TrimSpace(cfg.WebhookPublicURL) != "" {
			return "webhook"
		}
		return "polling"
	}
	return mode
}

func wechatProviderFromConfig(cfg *config.WeChatConfig) string {
	if cfg == nil {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		return "wecom"
	}
	return provider
}

func wechatModeFromConfig(cfg *config.WeChatConfig) string {
	if cfg == nil {
		return ""
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode != "" && mode != "auto" {
		return mode
	}
	if strings.TrimSpace(cfg.BaseURL) != "" || strings.TrimSpace(cfg.Token) != "" || strings.TrimSpace(cfg.StateFile) != "" || cfg.PollIntervalSeconds > 0 {
		return "polling"
	}
	return "callback"
}

func wechatCallbackStatusFromConfig(cfg *config.WeChatConfig) *callbackStatus {
	if cfg == nil {
		return nil
	}
	return &callbackStatus{
		ListenAddrConfigured: strings.TrimSpace(cfg.CallbackListenAddr) != "",
		PathConfigured:       strings.TrimSpace(cfg.CallbackPath) != "",
		TokenConfigured:      strings.TrimSpace(cfg.CallbackToken) != "",
		AESKeyConfigured:     strings.TrimSpace(cfg.CallbackEncodingAESKey) != "",
	}
}

func wechatPollingStatusFromConfig(cfg *config.WeChatConfig) *pollingStatus {
	if cfg == nil {
		return nil
	}
	return &pollingStatus{
		BaseURLConfigured:   strings.TrimSpace(cfg.BaseURL) != "",
		TokenConfigured:     strings.TrimSpace(cfg.Token) != "",
		StateFileConfigured: strings.TrimSpace(cfg.StateFile) != "",
		IntervalSeconds:     cfg.PollIntervalSeconds,
	}
}

func telegramWebhookStatusFromConfig(cfg *config.TelegramConfig) *channels.TelegramWebhookStatus {
	if cfg == nil {
		return nil
	}
	return &channels.TelegramWebhookStatus{
		RegisterOnStart:      cfg.WebhookRegisterOnStart,
		DeleteOnStop:         cfg.WebhookDeleteOnStop,
		PublicURLConfigured:  strings.TrimSpace(cfg.WebhookPublicURL) != "",
		ListenAddrConfigured: strings.TrimSpace(cfg.WebhookListenAddr) != "",
	}
}

func formatStatusTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
func (s *Server) agentStatus() *agentStatus {
	if s.config == nil || s.config.Agents == nil {
		return nil
	}

	status := &agentStatus{
		WorkspaceConfigured: strings.TrimSpace(s.config.Agents.Workspace) != "",
		MaxConcurrent:       s.config.Agents.MaxConcurrent,
	}

	if s.config.Agents.OhMyCode != nil {
		ohMyCode := s.config.Agents.OhMyCode
		status.OhMyCode = &ohMyCodeStatus{
			Enabled:             ohMyCode.Enabled,
			WorkspaceConfigured: strings.TrimSpace(ohMyCode.Workspace) != "",
			DefaultAgent:        strings.TrimSpace(ohMyCode.DefaultAgent),
		}
		if len(ohMyCode.AllowedAgents) > 0 {
			status.OhMyCode.AllowedAgents = append([]string{}, ohMyCode.AllowedAgents...)
		}
		if s.agentManager != nil {
			if routing := s.agentManager.LastRoutingOutcome(); routing != nil {
				status.OhMyCode.LastRouting = &ohMyCodeRoutingStatus{
					SelectedAgent: routing.SelectedAgent,
					Channel:       routing.Channel,
					ChatID:        routing.ChatID,
					UserID:        routing.UserID,
					Username:      routing.Username,
					Status:        routing.Status,
					Error:         routing.Error,
					RecordedAt:    formatStatusTime(routing.RecordedAt),
				}
			}
		}
	}

	return status
}

func (s *Server) snapshotClients() []*Client {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	clients := make([]*Client, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	return clients
}

func (s *Server) removeClient(id string) {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()
	delete(s.clients, id)
}
