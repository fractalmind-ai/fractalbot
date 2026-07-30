package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/channels"
	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
	"github.com/gorilla/websocket"
)

func TestGatewayEchoAndStatus(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Bind: "127.0.0.1",
			Port: 0,
		},
		Channels: &config.ChannelsConfig{},
		Agents:   &config.AgentsConfig{},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	server.startTime = time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.handleWebSocket)
	mux.HandleFunc("/status", server.handleStatus)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	msg := protocol.Message{
		Kind:   protocol.MessageKindEvent,
		Action: protocol.ActionEcho,
		Data: map[string]string{
			"text": "hello",
		},
	}

	if err := conn.WriteJSON(&msg); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var resp protocol.Message
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}

	if resp.Kind != protocol.MessageKindEvent || resp.Action != protocol.ActionEcho {
		t.Fatalf("unexpected response: %#v", resp)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %#v", resp.Data)
	}
	if data["text"] != "hello" {
		t.Fatalf("unexpected echo payload: %#v", data)
	}

	if err := waitForActiveClients(server, 1, time.Second); err != nil {
		t.Fatalf("active clients not tracked: %v", err)
	}

	statusResp, err := fetchStatus(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	if statusResp.Status != "ok" {
		t.Fatalf("unexpected status: %#v", statusResp)
	}
	if statusResp.ActiveClients != 1 {
		t.Fatalf("unexpected active_clients: %d", statusResp.ActiveClients)
	}
	if statusResp.Uptime == "" {
		t.Fatalf("expected uptime in status response")
	}

	_ = conn.Close()

	if err := waitForActiveClients(server, 0, time.Second); err != nil {
		t.Fatalf("client cleanup failed: %v", err)
	}
}

type statusPayload struct {
	Status        string `json:"status"`
	ActiveClients int    `json:"active_clients"`
	Uptime        string `json:"uptime"`
	Channels      []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Running bool   `json:"running"`
		Mode    string `json:"mode"`
		Webhook *struct {
			RegisterOnStart      bool `json:"register_on_start"`
			DeleteOnStop         bool `json:"delete_on_stop"`
			PublicURLConfigured  bool `json:"public_url_configured"`
			ListenAddrConfigured bool `json:"listen_addr_configured"`
			Registered           bool `json:"registered"`
		} `json:"webhook"`
		LastError    string `json:"last_error"`
		LastActivity string `json:"last_activity"`
	} `json:"channels"`
	Agents *struct {
		WorkspaceConfigured bool `json:"workspace_configured"`
		MaxConcurrent       int  `json:"max_concurrent"`
		OhMyCode            *struct {
			Enabled             bool     `json:"enabled"`
			WorkspaceConfigured bool     `json:"workspace_configured"`
			DefaultAgent        string   `json:"default_agent"`
			AllowedAgents       []string `json:"allowed_agents"`
			LastRouting         *struct {
				SelectedAgent string `json:"selected_agent"`
				Channel       string `json:"channel"`
				ChatID        string `json:"chat_id"`
				UserID        string `json:"user_id"`
				Username      string `json:"username"`
				Status        string `json:"status"`
				Error         string `json:"error"`
				RecordedAt    string `json:"recorded_at"`
			} `json:"last_routing"`
		} `json:"oh_my_code"`
		ClaudeDesktop *struct {
			Enabled          bool     `json:"enabled"`
			CDPEndpoint      string   `json:"cdp_endpoint"`
			TargetSelector   string   `json:"target_selector"`
			InboxConfigured  bool     `json:"inbox_configured"`
			FallbackToInbox  bool     `json:"fallback_to_inbox"`
			DefaultAgent     string   `json:"default_agent"`
			AllowedAgents    []string `json:"allowed_agents"`
			DeliveryTimeoutS int      `json:"delivery_timeout_seconds"`
		} `json:"claude_desktop"`
	} `json:"agents"`
}

func fetchStatus(url string) (*statusPayload, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var payload statusPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func waitForActiveClients(server *Server, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if server.activeClients() == want {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("active clients did not reach %d", want)
}

func TestStatusIncludesChannelAndAgentInfo(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Bind: "127.0.0.1",
			Port: 0,
		},
		Channels: &config.ChannelsConfig{
			Telegram: &config.TelegramConfig{
				Enabled:                true,
				Mode:                   "webhook",
				WebhookListenAddr:      "0.0.0.0:18790",
				WebhookPublicURL:       "https://example.com/telegram/webhook",
				WebhookRegisterOnStart: true,
				WebhookDeleteOnStop:    true,
			},
		},
		Agents: &config.AgentsConfig{
			Workspace:     "/tmp/agents",
			MaxConcurrent: 3,
			OhMyCode: &config.OhMyCodeConfig{
				Enabled:       true,
				Workspace:     "/tmp/oh-my-code",
				DefaultAgent:  "qa-1",
				AllowedAgents: []string{"qa-1", "coder-a"},
			},
		},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	statusResp, err := fetchStatus(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}

	if len(statusResp.Channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(statusResp.Channels))
	}
	if statusResp.Channels[0].Name != "telegram" {
		t.Fatalf("unexpected channel name: %s", statusResp.Channels[0].Name)
	}
	if !statusResp.Channels[0].Enabled {
		t.Fatalf("expected telegram to be enabled")
	}
	if statusResp.Channels[0].Running {
		t.Fatalf("expected telegram running=false before start")
	}
	if statusResp.Channels[0].Mode != "webhook" {
		t.Fatalf("unexpected telegram mode: %s", statusResp.Channels[0].Mode)
	}
	if statusResp.Channels[0].Webhook == nil {
		t.Fatalf("expected webhook status")
	}
	if !statusResp.Channels[0].Webhook.RegisterOnStart {
		t.Fatalf("expected webhook register_on_start true")
	}
	if !statusResp.Channels[0].Webhook.DeleteOnStop {
		t.Fatalf("expected webhook delete_on_stop true")
	}
	if !statusResp.Channels[0].Webhook.PublicURLConfigured {
		t.Fatalf("expected webhook public_url_configured true")
	}
	if !statusResp.Channels[0].Webhook.ListenAddrConfigured {
		t.Fatalf("expected webhook listen_addr_configured true")
	}

	if statusResp.Agents == nil {
		t.Fatalf("expected agents info")
	}
	if !statusResp.Agents.WorkspaceConfigured {
		t.Fatalf("expected workspace configured")
	}
	if statusResp.Agents.MaxConcurrent != 3 {
		t.Fatalf("unexpected max_concurrent: %d", statusResp.Agents.MaxConcurrent)
	}
	if statusResp.Agents.OhMyCode == nil {
		t.Fatalf("expected oh_my_code info")
	}
	if !statusResp.Agents.OhMyCode.Enabled {
		t.Fatalf("expected oh_my_code enabled")
	}
	if statusResp.Agents.OhMyCode.DefaultAgent != "qa-1" {
		t.Fatalf("unexpected default_agent: %s", statusResp.Agents.OhMyCode.DefaultAgent)
	}
	if len(statusResp.Agents.OhMyCode.AllowedAgents) != 2 {
		t.Fatalf("unexpected allowed_agents: %v", statusResp.Agents.OhMyCode.AllowedAgents)
	}
}

type fakeTelemetryChannel struct {
	name         string
	running      bool
	lastError    time.Time
	lastActivity time.Time
}

func (f *fakeTelemetryChannel) Name() string {
	return f.name
}

func (f *fakeTelemetryChannel) Start(ctx context.Context) error {
	f.running = true
	return nil
}

func (f *fakeTelemetryChannel) Stop(ctx context.Context) error {
	_ = ctx
	f.running = false
	return nil
}

func (f *fakeTelemetryChannel) Send(ctx context.Context, msg channels.OutboundMessage) (*channels.SendResult, error) {
	_ = ctx
	return nil, nil
}

func (f *fakeTelemetryChannel) IsRunning() bool {
	return f.running
}

func (f *fakeTelemetryChannel) IsAllowed(senderID string) bool {
	return true
}

func (f *fakeTelemetryChannel) LastError() time.Time {
	return f.lastError
}

func (f *fakeTelemetryChannel) LastActivity() time.Time {
	return f.lastActivity
}

func TestStatusIncludesChannelTelemetry(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Bind: "127.0.0.1",
			Port: 0,
		},
		Channels: &config.ChannelsConfig{
			Telegram: &config.TelegramConfig{Enabled: true},
		},
		Agents: &config.AgentsConfig{},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	lastError := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	lastActivity := time.Date(2024, 1, 3, 4, 5, 6, 0, time.UTC)
	fake := &fakeTelemetryChannel{
		name:         "telegram",
		lastError:    lastError,
		lastActivity: lastActivity,
	}
	if err := server.agentManager.ChannelManager.Register(fake); err != nil {
		t.Fatalf("register fake channel: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	statusResp, err := fetchStatus(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	if len(statusResp.Channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(statusResp.Channels))
	}
	if got := statusResp.Channels[0].LastError; got != lastError.Format(time.RFC3339) {
		t.Fatalf("last_error=%q", got)
	}
	if got := statusResp.Channels[0].LastActivity; got != lastActivity.Format(time.RFC3339) {
		t.Fatalf("last_activity=%q", got)
	}
}

func TestStatusIncludesClaudeDesktopConfig(t *testing.T) {
	cfg := &config.Config{
		Gateway:  &config.GatewayConfig{Bind: "127.0.0.1", Port: 0},
		Channels: &config.ChannelsConfig{},
		Agents: &config.AgentsConfig{
			Router: "claudeDesktop",
			ClaudeDesktop: &config.ClaudeDesktopConfig{
				Enabled:                true,
				CDPEndpoint:            "http://127.0.0.1:19334",
				TargetSelector:         "Claude",
				InboxPath:              "/tmp/claude-inbox",
				FallbackToInbox:        true,
				DefaultAgent:           "main",
				AllowedAgents:          []string{"main"},
				DeliveryTimeoutSeconds: 20,
			},
		},
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	statusResp, err := fetchStatus(ts.URL + "/status")
	if err != nil {
		t.Fatalf("fetch status: %v", err)
	}
	claude := statusResp.Agents.ClaudeDesktop
	if claude == nil || !claude.Enabled || claude.CDPEndpoint != "http://127.0.0.1:19334" || claude.TargetSelector != "Claude" || !claude.InboxConfigured || !claude.FallbackToInbox || claude.DefaultAgent != "main" || claude.DeliveryTimeoutS != 20 {
		t.Fatalf("unexpected Claude Desktop status: %#v", claude)
	}
	if len(claude.AllowedAgents) != 1 || claude.AllowedAgents[0] != "main" {
		t.Fatalf("unexpected allowed agents: %#v", claude.AllowedAgents)
	}
}

func TestStatusDoesNotExposeSecrets(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Bind: "127.0.0.1",
			Port: 0,
		},
		Channels: &config.ChannelsConfig{
			Telegram: &config.TelegramConfig{
				Enabled:  true,
				BotToken: "bot-token-secret",
			},
			Feishu: &config.FeishuConfig{
				Enabled:   true,
				AppID:     "cli_secret",
				AppSecret: "app-secret",
			},
		},
		Agents: &config.AgentsConfig{},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	text := string(body)
	if strings.Contains(text, "bot-token-secret") || strings.Contains(text, "app-secret") || strings.Contains(text, "cli_secret") {
		t.Fatalf("status response leaked secrets: %s", text)
	}
}

func TestStatusIncludesOhMyCodeRoutingTelemetry(t *testing.T) {
	workspace := t.TempDir()
	scriptPath := filepath.Join(workspace, "agent_manager.py")

	script := `import sys

if len(sys.argv) >= 2 and sys.argv[1] == "assign":
    print("assign ok")
    sys.exit(0)

print("unexpected command", file=sys.stderr)
sys.exit(1)
`

	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Bind: "127.0.0.1",
			Port: 0,
		},
		Channels: &config.ChannelsConfig{},
		Agents: &config.AgentsConfig{
			Workspace: workspace,
			OhMyCode: &config.OhMyCodeConfig{
				Enabled:            true,
				Workspace:          workspace,
				AgentManagerScript: scriptPath,
				DefaultAgent:       "qa-1",
			},
		},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	reply, err := server.agentManager.HandleIncoming(context.Background(), &protocol.Message{
		Data: map[string]interface{}{
			"channel":  "telegram",
			"text":     "hello",
			"chat_id":  int64(321),
			"user_id":  int64(456),
			"username": "bob",
		},
	})
	if err != nil {
		t.Fatalf("HandleIncoming failed: %v", err)
	}
	if reply != "处理中…" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	statusResp, err := fetchStatus(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	if statusResp.Agents == nil || statusResp.Agents.OhMyCode == nil || statusResp.Agents.OhMyCode.LastRouting == nil {
		t.Fatalf("expected last_routing telemetry, got %#v", statusResp.Agents)
	}
	routing := statusResp.Agents.OhMyCode.LastRouting
	if routing.SelectedAgent != "qa-1" {
		t.Fatalf("selected_agent=%q", routing.SelectedAgent)
	}
	if routing.Channel != "telegram" || routing.ChatID != "321" || routing.UserID != "456" || routing.Username != "bob" {
		t.Fatalf("unexpected routing payload: %#v", routing)
	}
	if routing.Status != "assigned" {
		t.Fatalf("status=%q", routing.Status)
	}
	if routing.RecordedAt == "" {
		t.Fatal("expected recorded_at")
	}
}

func TestHealthEndpointReturnsJSON(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Bind: "127.0.0.1",
			Port: 0,
		},
		Channels: &config.ChannelsConfig{
			Telegram: &config.TelegramConfig{Enabled: true},
		},
		Agents: &config.AgentsConfig{},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	server.startTime = time.Now().Add(-5 * time.Second)

	fake := &fakeSendChannel{name: "telegram", running: true}
	if err := server.agentManager.ChannelManager.Register(fake); err != nil {
		t.Fatalf("register fake channel: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var payload struct {
		Status   string `json:"status"`
		Uptime   string `json:"uptime"`
		Channels []struct {
			Name    string `json:"name"`
			Running bool   `json:"running"`
		} `json:"channels"`
		MessagesProcessed int64 `json:"messages_processed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health response: %v", err)
	}

	if payload.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", payload.Status)
	}
	if payload.Uptime == "" || payload.Uptime == "0s" {
		t.Fatalf("expected non-zero uptime, got %q", payload.Uptime)
	}
	if len(payload.Channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(payload.Channels))
	}
	if payload.Channels[0].Name != "telegram" {
		t.Fatalf("expected channel name=telegram, got %q", payload.Channels[0].Name)
	}
	if !payload.Channels[0].Running {
		t.Fatalf("expected channel running=true")
	}
	if payload.MessagesProcessed != 0 {
		t.Fatalf("expected messages_processed=0, got %d", payload.MessagesProcessed)
	}
}

type fakeSendChannel struct {
	name       string
	running    bool
	lastChat   string
	lastText   string
	lastThread string
	sendErr    error
}

func (f *fakeSendChannel) Name() string { return f.name }

func (f *fakeSendChannel) Start(ctx context.Context) error {
	_ = ctx
	f.running = true
	return nil
}

func (f *fakeSendChannel) Stop(ctx context.Context) error {
	_ = ctx
	f.running = false
	return nil
}

func (f *fakeSendChannel) Send(ctx context.Context, msg channels.OutboundMessage) (*channels.SendResult, error) {
	_ = ctx
	f.lastChat = msg.To
	f.lastText = msg.Text
	f.lastThread = msg.ThreadTS
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return &channels.SendResult{ChannelID: msg.To}, nil
}

func (f *fakeSendChannel) IsRunning() bool { return f.running }

func (f *fakeSendChannel) IsAllowed(senderID string) bool { return true }

func TestMessageSendAPI(t *testing.T) {
	cfg := &config.Config{
		Gateway:  &config.GatewayConfig{Bind: "127.0.0.1", Port: 0},
		Channels: &config.ChannelsConfig{Telegram: &config.TelegramConfig{Enabled: true}},
		Agents:   &config.AgentsConfig{},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	fake := &fakeSendChannel{name: "telegram"}
	if err := server.agentManager.ChannelManager.Register(fake); err != nil {
		t.Fatalf("register fake channel: %v", err)
	}
	fakeSlack := &fakeSendChannel{name: "slack"}
	if err := server.agentManager.ChannelManager.Register(fakeSlack); err != nil {
		t.Fatalf("register fake slack channel: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/message/send", server.handleMessageSend)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	t.Run("success", func(t *testing.T) {
		resp, err := http.Post(
			ts.URL+"/api/v1/message/send",
			"application/json",
			strings.NewReader(`{"channel":"telegram","to":"12345","text":"hello from api"}`),
		)
		if err != nil {
			t.Fatalf("post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("unexpected status %d body=%s", resp.StatusCode, string(body))
		}

		if fake.lastChat != "12345" {
			t.Fatalf("expected lastChat=12345 got %s", fake.lastChat)
		}
		if fake.lastText != "hello from api" {
			t.Fatalf("expected text captured, got %q", fake.lastText)
		}

		var payload messageSendResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if payload.Status != "ok" || payload.Channel != "telegram" || payload.To != "12345" {
			t.Fatalf("unexpected response payload: %#v", payload)
		}
	})

	t.Run("success with thread ts", func(t *testing.T) {
		fakeSlack.lastChat = ""
		fakeSlack.lastText = ""
		fakeSlack.lastThread = ""

		resp, err := http.Post(
			ts.URL+"/api/v1/message/send",
			"application/json",
			strings.NewReader(`{"channel":"slack","to":"C0A8ESWV7D0","text":"reply","thread_ts":"1234567890.123456"}`),
		)
		if err != nil {
			t.Fatalf("post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("unexpected status %d body=%s", resp.StatusCode, string(body))
		}
		if fakeSlack.lastChat != "C0A8ESWV7D0" {
			t.Fatalf("expected lastChat=C0A8ESWV7D0 got %s", fakeSlack.lastChat)
		}
		if fakeSlack.lastText != "reply" {
			t.Fatalf("expected text captured, got %q", fakeSlack.lastText)
		}
		if fakeSlack.lastThread != "1234567890.123456" {
			t.Fatalf("expected thread ts passed, got %q", fakeSlack.lastThread)
		}
	})

	t.Run("thread ts tolerated for non-threaded channel", func(t *testing.T) {
		fake.lastChat = ""
		fake.lastText = ""

		resp, err := http.Post(
			ts.URL+"/api/v1/message/send",
			"application/json",
			strings.NewReader(`{"channel":"telegram","to":"98765","text":"hello","thread_ts":"1234567890.123456"}`),
		)
		if err != nil {
			t.Fatalf("post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("unexpected status %d body=%s", resp.StatusCode, string(body))
		}
		if fake.lastChat != "98765" {
			t.Fatalf("expected lastChat=98765 got %s", fake.lastChat)
		}
		if fake.lastText != "hello" {
			t.Fatalf("expected text captured, got %q", fake.lastText)
		}
	})

	t.Run("validation", func(t *testing.T) {
		resp, err := http.Post(
			ts.URL+"/api/v1/message/send",
			"application/json",
			strings.NewReader(`{"channel":"telegram","to":"","text":""}`),
		)
		if err != nil {
			t.Fatalf("post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, string(body))
		}
	})

	t.Run("not found", func(t *testing.T) {
		resp, err := http.Post(
			ts.URL+"/api/v1/message/send",
			"application/json",
			strings.NewReader(`{"channel":"unknown-channel","to":"1","text":"hello"}`),
		)
		if err != nil {
			t.Fatalf("post failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d body=%s", resp.StatusCode, string(body))
		}
	})
}

func TestStatusEndpointReportsCodexAppCDPRouting(t *testing.T) {
	inbox := filepath.Join(t.TempDir(), "inbox")
	cfg := &config.Config{
		Gateway:  &config.GatewayConfig{Bind: "127.0.0.1", Port: 0},
		Channels: &config.ChannelsConfig{},
		Agents: &config.AgentsConfig{
			Router: "codexAppCDP",
			CodexAppCDP: &config.CodexAppCDPConfig{
				Enabled:         true,
				TargetSelector:  "Codex",
				HostID:          "local",
				InboxPath:       inbox,
				FallbackToInbox: true,
				RepairPolicy:    "status-only",
				DefaultAgent:    "main",
				AllowedAgents:   []string{"main"},
			},
		},
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	_, err = server.agentManager.HandleIncoming(context.Background(), &protocol.Message{Data: map[string]interface{}{
		"channel": "feishu",
		"text":    "hello",
		"agent":   "main",
		"chat_id": "oc_1",
		"open_id": "ou_1",
	}})
	if err != nil {
		t.Fatalf("HandleIncoming failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	agents := payload["agents"].(map[string]interface{})
	if agents["router"] != "codexAppCDP" {
		t.Fatalf("router=%v", agents["router"])
	}
	codex := agents["codex_app_cdp"].(map[string]interface{})
	if codex["enabled"] != true || codex["default_agent"] != "main" || codex["inbox_configured"] != true {
		t.Fatalf("unexpected codex status: %#v", codex)
	}
	if codex["repair_policy"] != "status-only" || codex["check_on_incoming_message"] != true {
		t.Fatalf("unexpected codex readiness config: %#v", codex)
	}
	routing := agents["last_routing"].(map[string]interface{})
	if routing["backend"] != "codexAppCDP" || routing["status"] != "queued" || routing["envelope_id"] == "" {
		t.Fatalf("unexpected routing status: %#v", routing)
	}
}

func TestStatusEndpointReportsCodexAppCDPDefaultRepairAndWatch(t *testing.T) {
	cfg := &config.Config{
		Gateway:  &config.GatewayConfig{Bind: "127.0.0.1", Port: 0},
		Channels: &config.ChannelsConfig{},
		Agents: &config.AgentsConfig{
			Router: "codexAppCDP",
			CodexAppCDP: &config.CodexAppCDPConfig{
				Enabled:      true,
				CDPEndpoint:  "http://127.0.0.1:9222",
				DefaultAgent: "main",
				TargetProject: config.CodexAppCDPTargetProjectConfig{
					Name:    "CloudBank",
					CWD:     "/repo/cloudbank",
					Session: "main",
				},
			},
		},
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	agents := payload["agents"].(map[string]interface{})
	codex := agents["codex_app_cdp"].(map[string]interface{})
	if codex["repair_policy"] != "relaunch" {
		t.Fatalf("expected relaunch default, got %#v", codex)
	}
	watch := codex["watch"].(map[string]interface{})
	if watch["enabled"] != true {
		t.Fatalf("expected watch enabled by default, got %#v", watch)
	}
	targetProject := codex["target_project"].(map[string]interface{})
	if targetProject["name"] != "CloudBank" || targetProject["cwd"] != "/repo/cloudbank" || targetProject["session"] != "main" {
		t.Fatalf("unexpected target_project: %#v", targetProject)
	}
}

func TestHeartbeatCronAPIAndStatus(t *testing.T) {
	inbox := filepath.Join(t.TempDir(), "inbox")
	cfg := heartbeatGatewayConfig(t, inbox)
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/heartbeat/jobs/", server.handleHeartbeatCron)
	mux.HandleFunc("/status", server.handleStatus)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	request, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/heartbeat/jobs/cloudbank-main/cron", strings.NewReader(`{"profile":"idle","reason":"no actionable tasks"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("set profile request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("set profile status=%d body=%s", response.StatusCode, body)
	}
	var setPayload heartbeatCronResponse
	if err := json.NewDecoder(response.Body).Decode(&setPayload); err != nil {
		t.Fatalf("decode set response: %v", err)
	}
	if setPayload.Job == nil || setPayload.Job.EffectiveProfile != "idle" || setPayload.Job.EffectiveCron != "0 * * * *" {
		t.Fatalf("unexpected set response: %#v", setPayload)
	}

	statusResponse, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	statusData, err := io.ReadAll(statusResponse.Body)
	statusResponse.Body.Close()
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), `"effective_profile":"idle"`) || !strings.Contains(string(statusData), `"last_schedule_reason":"no actionable tasks"`) {
		t.Fatalf("status missing heartbeat profile: %s", statusData)
	}
	if strings.Contains(string(statusData), "secret heartbeat instruction") {
		t.Fatalf("status leaked heartbeat instruction: %s", statusData)
	}

	resetRequest, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/heartbeat/jobs/cloudbank-main/cron", nil)
	if err != nil {
		t.Fatal(err)
	}
	resetResponse, err := http.DefaultClient.Do(resetRequest)
	if err != nil {
		t.Fatalf("reset profile request: %v", err)
	}
	defer resetResponse.Body.Close()
	if resetResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resetResponse.Body)
		t.Fatalf("reset profile status=%d body=%s", resetResponse.StatusCode, body)
	}
	var resetPayload heartbeatCronResponse
	if err := json.NewDecoder(resetResponse.Body).Decode(&resetPayload); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if resetPayload.Job == nil || resetPayload.Job.EffectiveProfile != "" || resetPayload.Job.EffectiveCron != "*/10 * * * *" {
		t.Fatalf("unexpected reset response: %#v", resetPayload)
	}
}

func TestHeartbeatCronAPIRejectsInvalidAndRemoteRequests(t *testing.T) {
	server, err := NewServer(heartbeatGatewayConfig(t, filepath.Join(t.TempDir(), "inbox")))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	t.Run("invalid profile", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/heartbeat/jobs/cloudbank-main/cron", strings.NewReader(`{"profile":"arbitrary","reason":"idle"}`))
		request.RemoteAddr = "127.0.0.1:12345"
		response := httptest.NewRecorder()
		server.handleHeartbeatCron(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "not allowed") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("missing reason", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/heartbeat/jobs/cloudbank-main/cron", strings.NewReader(`{"profile":"idle"}`))
		request.RemoteAddr = "[::1]:12345"
		response := httptest.NewRecorder()
		server.handleHeartbeatCron(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "reason is required") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("remote client", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/heartbeat/jobs/cloudbank-main/cron", strings.NewReader(`{"profile":"idle","reason":"idle"}`))
		request.RemoteAddr = "203.0.113.20:4567"
		response := httptest.NewRecorder()
		server.handleHeartbeatCron(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func TestHeartbeatProfileResetsAfterMatchingInboundRoute(t *testing.T) {
	server, err := NewServer(heartbeatGatewayConfig(t, filepath.Join(t.TempDir(), "inbox")))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if _, err := server.heartbeat.SetProfile("cloudbank-main", "idle", "no actionable tasks", "agent"); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}

	_, err = server.agentManager.HandleIncoming(context.Background(), &protocol.Message{Data: map[string]interface{}{
		"channel": "feishu",
		"text":    "new user task",
		"chat_id": "oc_1",
	}})
	if err != nil {
		t.Fatalf("HandleIncoming: %v", err)
	}
	job := server.heartbeat.Status().Jobs[0]
	if job.EffectiveProfile != "" || job.LastScheduleReason != "normal inbound activity" {
		t.Fatalf("matching inbound did not reset heartbeat: %#v", job)
	}
}

func heartbeatGatewayConfig(t *testing.T, inbox string) *config.Config {
	t.Helper()
	return &config.Config{
		Gateway:  &config.GatewayConfig{Bind: "127.0.0.1", Port: 0},
		Channels: &config.ChannelsConfig{},
		Agents: &config.AgentsConfig{
			Workspace: t.TempDir(),
			Router:    "codexAppCDP",
			CodexAppCDP: &config.CodexAppCDPConfig{
				Enabled:       true,
				InboxPath:     inbox,
				DefaultAgent:  "main",
				AllowedAgents: []string{"main"},
			},
			Heartbeat: &config.HeartbeatConfig{
				Enabled:       true,
				MaxConcurrent: 1,
				Jobs: []config.HeartbeatJobConfig{{
					ID:       "cloudbank-main",
					Runtime:  "codexAppCDP",
					Agent:    "main",
					Text:     "secret heartbeat instruction",
					Cron:     "*/10 * * * *",
					Timezone: "Asia/Shanghai",
					AgentCronProfiles: map[string]string{
						"idle": "0 * * * *",
					},
					ResetCronOnInbound: true,
				}},
			},
		},
	}
}
