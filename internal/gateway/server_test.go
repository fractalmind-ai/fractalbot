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
		Name     string `json:"name"`
		Enabled  bool   `json:"enabled"`
		Running  bool   `json:"running"`
		Mode     string `json:"mode"`
		Provider string `json:"provider"`
		Webhook  *struct {
			RegisterOnStart      bool `json:"register_on_start"`
			DeleteOnStop         bool `json:"delete_on_stop"`
			PublicURLConfigured  bool `json:"public_url_configured"`
			ListenAddrConfigured bool `json:"listen_addr_configured"`
			Registered           bool `json:"registered"`
		} `json:"webhook"`
		Callback *struct {
			ListenAddrConfigured bool `json:"listen_addr_configured"`
			PathConfigured       bool `json:"path_configured"`
			TokenConfigured      bool `json:"token_configured"`
			AESKeyConfigured     bool `json:"aes_key_configured"`
		} `json:"callback"`
		Polling *struct {
			BaseURLConfigured   bool   `json:"base_url_configured"`
			TokenConfigured     bool   `json:"token_configured"`
			StateFileConfigured bool   `json:"state_file_configured"`
			IntervalSeconds     int    `json:"interval_seconds"`
			CursorPresent       bool   `json:"cursor_present"`
			CursorPreview       string `json:"cursor_preview"`
			StateFileExists     bool   `json:"state_file_exists"`
			LastPollAt          string `json:"last_poll_at"`
			LastPollMessages    int    `json:"last_poll_messages"`
		} `json:"polling"`
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
			WeChat: &config.WeChatConfig{
				Enabled:                true,
				Provider:               "wecom",
				Mode:                   "polling",
				BaseURL:                "https://ilinkai.weixin.qq.com/",
				Token:                  "bot-token-123",
				StateFile:              "./workspace/wechat.polling.state.json",
				PollIntervalSeconds:    3,
				CallbackListenAddr:     "127.0.0.1:18810",
				CallbackPath:           "/wechat/callback",
				CallbackToken:          "token-123",
				CallbackEncodingAESKey: "aes-key-123",
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

	if len(statusResp.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(statusResp.Channels))
	}
	channelsByName := map[string]struct {
		Enabled  bool
		Running  bool
		Mode     string
		Provider string
		Webhook  *struct {
			RegisterOnStart      bool `json:"register_on_start"`
			DeleteOnStop         bool `json:"delete_on_stop"`
			PublicURLConfigured  bool `json:"public_url_configured"`
			ListenAddrConfigured bool `json:"listen_addr_configured"`
			Registered           bool `json:"registered"`
		}
		Callback *struct {
			ListenAddrConfigured bool `json:"listen_addr_configured"`
			PathConfigured       bool `json:"path_configured"`
			TokenConfigured      bool `json:"token_configured"`
			AESKeyConfigured     bool `json:"aes_key_configured"`
		}
		Polling *struct {
			BaseURLConfigured   bool   `json:"base_url_configured"`
			TokenConfigured     bool   `json:"token_configured"`
			StateFileConfigured bool   `json:"state_file_configured"`
			IntervalSeconds     int    `json:"interval_seconds"`
			CursorPresent       bool   `json:"cursor_present"`
			CursorPreview       string `json:"cursor_preview"`
			StateFileExists     bool   `json:"state_file_exists"`
			LastPollAt          string `json:"last_poll_at"`
			LastPollMessages    int    `json:"last_poll_messages"`
		}
	}{}
	for _, ch := range statusResp.Channels {
		channelsByName[ch.Name] = struct {
			Enabled  bool
			Running  bool
			Mode     string
			Provider string
			Webhook  *struct {
				RegisterOnStart      bool `json:"register_on_start"`
				DeleteOnStop         bool `json:"delete_on_stop"`
				PublicURLConfigured  bool `json:"public_url_configured"`
				ListenAddrConfigured bool `json:"listen_addr_configured"`
				Registered           bool `json:"registered"`
			}
			Callback *struct {
				ListenAddrConfigured bool `json:"listen_addr_configured"`
				PathConfigured       bool `json:"path_configured"`
				TokenConfigured      bool `json:"token_configured"`
				AESKeyConfigured     bool `json:"aes_key_configured"`
			}
			Polling *struct {
				BaseURLConfigured   bool   `json:"base_url_configured"`
				TokenConfigured     bool   `json:"token_configured"`
				StateFileConfigured bool   `json:"state_file_configured"`
				IntervalSeconds     int    `json:"interval_seconds"`
				CursorPresent       bool   `json:"cursor_present"`
				CursorPreview       string `json:"cursor_preview"`
				StateFileExists     bool   `json:"state_file_exists"`
				LastPollAt          string `json:"last_poll_at"`
				LastPollMessages    int    `json:"last_poll_messages"`
			}
		}{
			Enabled:  ch.Enabled,
			Running:  ch.Running,
			Mode:     ch.Mode,
			Provider: ch.Provider,
			Webhook:  ch.Webhook,
			Callback: ch.Callback,
			Polling:  ch.Polling,
		}
	}
	telegram := channelsByName["telegram"]
	if !telegram.Enabled {
		t.Fatalf("expected telegram to be enabled")
	}
	if telegram.Running {
		t.Fatalf("expected telegram running=false before start")
	}
	if telegram.Mode != "webhook" {
		t.Fatalf("unexpected telegram mode: %s", telegram.Mode)
	}
	if telegram.Webhook == nil {
		t.Fatalf("expected webhook status")
	}
	if !telegram.Webhook.RegisterOnStart {
		t.Fatalf("expected webhook register_on_start true")
	}
	if !telegram.Webhook.DeleteOnStop {
		t.Fatalf("expected webhook delete_on_stop true")
	}
	if !telegram.Webhook.PublicURLConfigured {
		t.Fatalf("expected webhook public_url_configured true")
	}
	if !telegram.Webhook.ListenAddrConfigured {
		t.Fatalf("expected webhook listen_addr_configured true")
	}

	wechat := channelsByName["wechat"]
	if !wechat.Enabled {
		t.Fatalf("expected wechat to be enabled")
	}
	if wechat.Running {
		t.Fatalf("expected wechat running=false before start")
	}
	if wechat.Provider != "wecom" {
		t.Fatalf("unexpected wechat provider: %s", wechat.Provider)
	}
	if wechat.Mode != "polling" {
		t.Fatalf("unexpected wechat mode: %s", wechat.Mode)
	}
	if wechat.Callback == nil {
		t.Fatalf("expected callback status")
	}
	if !wechat.Callback.ListenAddrConfigured || !wechat.Callback.PathConfigured || !wechat.Callback.TokenConfigured || !wechat.Callback.AESKeyConfigured {
		t.Fatalf("unexpected wechat callback status: %#v", wechat.Callback)
	}
	if wechat.Polling == nil {
		t.Fatalf("expected polling status")
	}
	if !wechat.Polling.BaseURLConfigured || !wechat.Polling.TokenConfigured || !wechat.Polling.StateFileConfigured || wechat.Polling.IntervalSeconds != 3 {
		t.Fatalf("unexpected wechat polling status: %#v", wechat.Polling)
	}
	if wechat.Polling.CursorPresent || wechat.Polling.CursorPreview != "" || wechat.Polling.StateFileExists || wechat.Polling.LastPollAt != "" || wechat.Polling.LastPollMessages != 0 {
		t.Fatalf("expected empty runtime polling status before start: %#v", wechat.Polling)
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

func TestStatusIncludesWeChatPollingRuntimeInfo(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ret":0,"errcode":0,"get_updates_buf":"cursor-abc-123","msgs":[{"client_id":"client-1","from_user_id":"wx-user","to_user_id":"wx-bot","session_id":"session-1","context_token":"ctx-1","item_list":[{"type":1,"text_item":{"text":"hello"}}]}]}`)
	}))
	defer upstream.Close()

	stateFile := t.TempDir() + "/wechat.polling.state.json"
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{Bind: "127.0.0.1", Port: 0},
		Channels: &config.ChannelsConfig{
			WeChat: &config.WeChatConfig{
				Enabled:             true,
				Provider:            "wecom",
				Mode:                "polling",
				BaseURL:             upstream.URL,
				Token:               "bot-token-123",
				StateFile:           stateFile,
				PollIntervalSeconds: 3,
			},
		},
		Agents: &config.AgentsConfig{},
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	bot, err := channels.NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot failed: %v", err)
	}
	bot.ConfigurePolling(upstream.URL, "bot-token-123", stateFile, 3)
	bot.ConfigureMode("polling")
	if err := server.agentManager.ChannelManager.Register(bot); err != nil {
		t.Fatalf("register wechat bot failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bot.Start(ctx); err != nil {
		t.Fatalf("bot.Start failed: %v", err)
	}
	defer func() { _ = bot.Stop() }()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := bot.PollingRuntimeStatus()
		if status.CursorPresent && status.StateFileExists && !status.LastPollAt.IsZero() {
			break
		}
		time.Sleep(10 * time.Millisecond)
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
	polling := statusResp.Channels[0].Polling
	if polling == nil {
		t.Fatalf("expected polling status")
	}
	if !polling.CursorPresent || !polling.StateFileExists {
		t.Fatalf("expected runtime polling state, got %#v", polling)
	}
	if polling.CursorPreview != "cursor-abc-123" {
		t.Fatalf("unexpected cursor_preview: %q", polling.CursorPreview)
	}
	if polling.LastPollAt == "" {
		t.Fatalf("expected last_poll_at")
	}
	if polling.LastPollMessages != 1 {
		t.Fatalf("unexpected last_poll_messages: %d", polling.LastPollMessages)
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
