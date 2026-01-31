package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	} `json:"channels"`
	Agents *struct {
		WorkspaceConfigured bool `json:"workspace_configured"`
		MaxConcurrent       int  `json:"max_concurrent"`
		OhMyCode            *struct {
			Enabled             bool     `json:"enabled"`
			WorkspaceConfigured bool     `json:"workspace_configured"`
			DefaultAgent        string   `json:"default_agent"`
			AllowedAgents       []string `json:"allowed_agents"`
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
