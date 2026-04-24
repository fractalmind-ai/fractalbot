package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestManagerRegistersWeChatChannel(t *testing.T) {
	manager := NewManager(&config.ChannelsConfig{
		WeChat: &config.WeChatConfig{
			Enabled:              true,
			Provider:             "wecom",
			CallbackPath:         "/wechat/callback",
			CallbackToken:        "token-123",
			AccessTokenCacheFile: "./workspace/wechat.token.json",
		},
	}, nil)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = manager.Stop() }()

	waitForCondition(t, time.Second, func() bool {
		channel := manager.Get("wechat")
		return channel != nil && channel.IsRunning()
	})
}

func TestManagerRegistersWeChatPollingMode(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(wechatPollingResponse{Ret: 0, Errcode: 0})
	}))
	defer server.Close()

	manager := NewManager(&config.ChannelsConfig{
		WeChat: &config.WeChatConfig{
			Enabled:             true,
			Provider:            "wecom",
			Mode:                "polling",
			BaseURL:             server.URL,
			Token:               "bot-token-123",
			StateFile:           "./workspace/wechat.polling.state.json",
			PollIntervalSeconds: 3,
			CallbackListenAddr:  "127.0.0.1:18810",
		},
	}, nil)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = manager.Stop() }()

	waitForCondition(t, time.Second, func() bool {
		channel := manager.Get("wechat")
		return channel != nil && channel.IsRunning()
	})
	waitForCondition(t, time.Second, func() bool {
		return requests > 0
	})

	channel := manager.Get("wechat")
	bot, ok := channel.(*WeChatBot)
	if !ok {
		t.Fatalf("expected WeChatBot, got %T", channel)
	}
	if bot.mode != wechatModePolling {
		t.Fatalf("mode=%q want %q", bot.mode, wechatModePolling)
	}
	if bot.pollingBaseURL != server.URL {
		t.Fatalf("pollingBaseURL=%q", bot.pollingBaseURL)
	}
	if bot.pollingToken != "bot-token-123" {
		t.Fatalf("pollingToken=%q", bot.pollingToken)
	}
	if bot.pollingStateFile != "./workspace/wechat.polling.state.json" {
		t.Fatalf("pollingStateFile=%q", bot.pollingStateFile)
	}
	if bot.pollingIntervalSeconds != 3 {
		t.Fatalf("pollingIntervalSeconds=%d want 3", bot.pollingIntervalSeconds)
	}
	if bot.callbackServer != nil {
		t.Fatalf("expected polling mode to skip callback server startup")
	}
}

func TestManagerRejectsInvalidWeChatProvider(t *testing.T) {
	manager := NewManager(&config.ChannelsConfig{
		WeChat: &config.WeChatConfig{
			Enabled:  true,
			Provider: "bad-provider",
		},
	}, nil)

	err := manager.Start(context.Background())
	if err == nil {
		t.Fatalf("expected invalid provider error")
	}
	if !strings.Contains(err.Error(), "invalid wechat provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}
