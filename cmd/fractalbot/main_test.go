package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
)

func TestRunMissingConfig(t *testing.T) {
	var buf bytes.Buffer
	code := runWithContext(context.Background(), []string{"--config", "/nope/config.yaml"}, &buf)
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	out := buf.String()
	if !strings.Contains(out, "failed to load config") && !strings.Contains(out, "failed to read config") {
		t.Fatalf("unexpected error output: %q", out)
	}
}

func TestRunMinimalConfigExitsOnCancel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	configContents := []byte("gateway:\n  bind: 127.0.0.1\n  port: 0\n")
	if err := os.WriteFile(configPath, configContents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	var buf bytes.Buffer
	code := runWithContext(ctx, []string{"--config", configPath}, &buf)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d output=%q", code, buf.String())
	}
}

func TestRunMessageSend(t *testing.T) {
	configPath := writeMinimalConfig(t)
	original := messageSendFn
	t.Cleanup(func() { messageSendFn = original })

	t.Run("success", func(t *testing.T) {
		called := false
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
			_ = ctx
			_ = cfg
			called = true
			if channel != "telegram" {
				t.Fatalf("channel=%q", channel)
			}
			if to != "5088760910" {
				t.Fatalf("to=%s", to)
			}
			if text != "hello from cli" {
				t.Fatalf("text=%q", text)
			}
			if threadTS != "" {
				t.Fatalf("threadTS=%q", threadTS)
			}
			return nil
		}

		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"message", "send",
			"--channel", "telegram",
			"--to", "5088760910",
			"--text", "hello from cli",
		}, &buf)

		if code != 0 {
			t.Fatalf("expected exit code 0, got %d output=%q", code, buf.String())
		}
		if !called {
			t.Fatalf("expected message send function to be called")
		}
		if !strings.Contains(buf.String(), "Message sent via telegram to 5088760910") {
			t.Fatalf("unexpected output: %q", buf.String())
		}
	})

	t.Run("success with thread ts", func(t *testing.T) {
		called := false
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
			_ = ctx
			_ = cfg
			called = true
			if channel != "slack" {
				t.Fatalf("channel=%q", channel)
			}
			if to != "C0A8ESWV7D0" {
				t.Fatalf("to=%s", to)
			}
			if text != "thread reply" {
				t.Fatalf("text=%q", text)
			}
			if threadTS != "1234567890.123456" {
				t.Fatalf("threadTS=%q", threadTS)
			}
			return nil
		}

		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"message", "send",
			"--channel", "slack",
			"--to", "C0A8ESWV7D0",
			"--thread-ts", "1234567890.123456",
			"--text", "thread reply",
		}, &buf)

		if code != 0 {
			t.Fatalf("expected exit code 0, got %d output=%q", code, buf.String())
		}
		if !called {
			t.Fatalf("expected message send function to be called")
		}
	})

	t.Run("success with repeatable images and text caption", func(t *testing.T) {
		var gotImages []string
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
			_ = ctx
			_ = cfg
			gotImages = images
			if channel != "feishu" {
				t.Fatalf("channel=%q", channel)
			}
			if to != "oc_1" {
				t.Fatalf("to=%s", to)
			}
			if text != "架构对比图" {
				t.Fatalf("text=%q", text)
			}
			if len(images) != 2 {
				t.Fatalf("expected 2 images, got %v", images)
			}
			return nil
		}

		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"message", "send",
			"--channel", "feishu",
			"--to", "oc_1",
			"--text", "架构对比图",
			"--image", "/tmp/arch-compare.png",
			"--image", "/tmp/loss-curve.png",
		}, &buf)

		if code != 0 {
			t.Fatalf("expected exit code 0, got %d output=%q", code, buf.String())
		}
		if len(gotImages) != 2 {
			t.Fatalf("expected images forwarded to send fn, got %v", gotImages)
		}
	})

	t.Run("images only (no text)", func(t *testing.T) {
		called := false
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
			_ = ctx
			_ = cfg
			called = true
			if text != "" {
				t.Fatalf("expected empty text, got %q", text)
			}
			if len(images) != 1 || images[0] != "/tmp/a.png" {
				t.Fatalf("unexpected images: %v", images)
			}
			return nil
		}

		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"message", "send",
			"--channel", "feishu",
			"--to", "oc_2",
			"--image", "/tmp/a.png",
		}, &buf)

		if code != 0 {
			t.Fatalf("expected exit code 0, got %d output=%q", code, buf.String())
		}
		if !called {
			t.Fatalf("expected message send function to be called")
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		cases := []struct {
			name          string
			args          []string
			expectedError string
		}{
			{
				name:          "missing to",
				args:          []string{"--channel", "telegram", "--text", "hello"},
				expectedError: "--to is required",
			},
			{
				name:          "missing text and image",
				args:          []string{"--channel", "telegram", "--to", "5088760910"},
				expectedError: "--text or --image is required",
			},
			{
				name:          "svg image rejected",
				args:          []string{"--channel", "feishu", "--to", "oc_1", "--text", "caption", "--image", "/tmp/chart.svg"},
				expectedError: "does not support SVG",
			},
		}

		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
					_ = ctx
					_ = cfg
					_ = channel
					_ = to
					_ = text
					_ = threadTS
					t.Fatalf("messageSendFn should not be called for validation error")
					return nil
				}

				var buf bytes.Buffer
				args := append([]string{"--config", configPath, "message", "send"}, testCase.args...)
				code := runWithContext(context.Background(), args, &buf)
				if code == 0 {
					t.Fatalf("expected non-zero exit code for %s", testCase.name)
				}
				if !strings.Contains(buf.String(), testCase.expectedError) {
					t.Fatalf("expected output to contain %q, got %q", testCase.expectedError, buf.String())
				}
			})
		}
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
			_ = ctx
			_ = cfg
			_ = channel
			_ = to
			_ = text
			_ = threadTS
			t.Fatalf("messageSendFn should not be called")
			return nil
		}

		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"message", "ping",
		}, &buf)
		if code == 0 {
			t.Fatalf("expected non-zero exit code")
		}
		if !strings.Contains(buf.String(), "unknown message subcommand") {
			t.Fatalf("unexpected output: %q", buf.String())
		}
	})

	t.Run("send failure", func(t *testing.T) {
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
			_ = ctx
			_ = cfg
			_ = channel
			_ = to
			_ = text
			_ = threadTS
			return fmt.Errorf("gateway down")
		}

		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"message", "send",
			"--channel", "telegram",
			"--to", "5088760910",
			"--text", "hello",
		}, &buf)

		if code == 0 {
			t.Fatalf("expected non-zero exit code")
		}
		if !strings.Contains(buf.String(), "failed to send message") {
			t.Fatalf("unexpected output: %q", buf.String())
		}
	})
}

func TestRunFileDownload(t *testing.T) {
	configPath := writeMinimalConfig(t)
	original := fileDownloadFn
	t.Cleanup(func() { fileDownloadFn = original })

	t.Run("success", func(t *testing.T) {
		called := false
		fileDownloadFn = func(ctx context.Context, cfg *config.Config, channel string, fileURL string, output string) error {
			_ = ctx
			_ = cfg
			called = true
			if channel != "slack" {
				t.Fatalf("channel=%q", channel)
			}
			if fileURL != "https://example.com/file.png" {
				t.Fatalf("url=%q", fileURL)
			}
			if output != "/tmp/file.png" {
				t.Fatalf("output=%q", output)
			}
			return nil
		}

		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"file", "download",
			"--channel", "slack",
			"--url", "https://example.com/file.png",
			"--output", "/tmp/file.png",
		}, &buf)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d output=%q", code, buf.String())
		}
		if !called {
			t.Fatalf("expected file download function to be called")
		}
		if !strings.Contains(buf.String(), "File downloaded via slack to /tmp/file.png") {
			t.Fatalf("unexpected output: %q", buf.String())
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		cases := []struct {
			name          string
			args          []string
			expectedError string
		}{
			{
				name:          "missing channel",
				args:          []string{"--url", "https://example.com/x", "--output", "/tmp/x"},
				expectedError: "--channel is required",
			},
			{
				name:          "missing url",
				args:          []string{"--channel", "slack", "--output", "/tmp/x"},
				expectedError: "--url is required",
			},
			{
				name:          "missing output",
				args:          []string{"--channel", "slack", "--url", "https://example.com/x"},
				expectedError: "--output is required",
			},
		}

		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				fileDownloadFn = func(ctx context.Context, cfg *config.Config, channel string, fileURL string, output string) error {
					_ = ctx
					_ = cfg
					_ = channel
					_ = fileURL
					_ = output
					t.Fatalf("fileDownloadFn should not be called for validation error")
					return nil
				}

				var buf bytes.Buffer
				args := append([]string{"--config", configPath, "file", "download"}, testCase.args...)
				code := runWithContext(context.Background(), args, &buf)
				if code == 0 {
					t.Fatalf("expected non-zero exit code for %s", testCase.name)
				}
				if !strings.Contains(buf.String(), testCase.expectedError) {
					t.Fatalf("expected output to contain %q, got %q", testCase.expectedError, buf.String())
				}
			})
		}
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		var buf bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"file", "inspect",
		}, &buf)
		if code == 0 {
			t.Fatalf("expected non-zero exit code")
		}
		if !strings.Contains(buf.String(), "unknown file subcommand") {
			t.Fatalf("unexpected output: %q", buf.String())
		}
	})
}

func TestRunHeartbeatCronCommands(t *testing.T) {
	configPath := writeMinimalConfig(t)
	originalSet := heartbeatCronSetFn
	originalReset := heartbeatCronResetFn
	t.Cleanup(func() {
		heartbeatCronSetFn = originalSet
		heartbeatCronResetFn = originalReset
	})

	t.Run("set", func(t *testing.T) {
		called := false
		heartbeatCronSetFn = func(ctx context.Context, cfg *config.Config, jobID, profile, reason string) error {
			called = true
			if jobID != "cloudbank-main" || profile != "idle" || reason != "no actionable tasks" {
				t.Fatalf("unexpected set arguments: job=%q profile=%q reason=%q", jobID, profile, reason)
			}
			return nil
		}
		var output bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"heartbeat", "cron", "set",
			"--job", "cloudbank-main",
			"--profile", "idle",
			"--reason", "no actionable tasks",
		}, &output)
		if code != 0 || !called {
			t.Fatalf("code=%d called=%t output=%q", code, called, output.String())
		}
		if !strings.Contains(output.String(), "now uses cron profile idle") {
			t.Fatalf("unexpected output: %q", output.String())
		}
	})

	t.Run("reset", func(t *testing.T) {
		called := false
		heartbeatCronResetFn = func(ctx context.Context, cfg *config.Config, jobID string) error {
			called = true
			if jobID != "cloudbank-main" {
				t.Fatalf("jobID=%q", jobID)
			}
			return nil
		}
		var output bytes.Buffer
		code := runWithContext(context.Background(), []string{
			"--config", configPath,
			"heartbeat", "cron", "reset",
			"--job", "cloudbank-main",
		}, &output)
		if code != 0 || !called {
			t.Fatalf("code=%d called=%t output=%q", code, called, output.String())
		}
		if !strings.Contains(output.String(), "restored to its default cron") {
			t.Fatalf("unexpected output: %q", output.String())
		}
	})

	t.Run("validation", func(t *testing.T) {
		for _, test := range []struct {
			name string
			args []string
			want string
		}{
			{name: "missing job", args: []string{"heartbeat", "cron", "set", "--profile", "idle", "--reason", "idle"}, want: "--job is required"},
			{name: "missing profile", args: []string{"heartbeat", "cron", "set", "--job", "job-1", "--reason", "idle"}, want: "--profile is required"},
			{name: "missing reason", args: []string{"heartbeat", "cron", "set", "--job", "job-1", "--profile", "idle"}, want: "--reason is required"},
			{name: "reset missing job", args: []string{"heartbeat", "cron", "reset"}, want: "--job is required"},
		} {
			t.Run(test.name, func(t *testing.T) {
				var output bytes.Buffer
				args := append([]string{"--config", configPath}, test.args...)
				if code := runWithContext(context.Background(), args, &output); code == 0 || !strings.Contains(output.String(), test.want) {
					t.Fatalf("code=%d output=%q want=%q", code, output.String(), test.want)
				}
			})
		}
	})
}

func TestHeartbeatCronGatewayAPIRequests(t *testing.T) {
	type observedRequest struct {
		method  string
		path    string
		profile string
		reason  string
	}
	observed := make(chan observedRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := observedRequest{method: r.Method, path: r.URL.Path}
		if r.Method == http.MethodPut {
			var payload heartbeatCronRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			request.profile = payload.Profile
			request.reason = payload.Reason
		}
		observed <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Gateway: &config.GatewayConfig{Bind: host, Port: port}}
	if err := setHeartbeatCronViaGatewayAPI(context.Background(), cfg, "cloudbank-main", "idle", "no actionable tasks"); err != nil {
		t.Fatalf("setHeartbeatCronViaGatewayAPI: %v", err)
	}
	if err := resetHeartbeatCronViaGatewayAPI(context.Background(), cfg, "cloudbank-main"); err != nil {
		t.Fatalf("resetHeartbeatCronViaGatewayAPI: %v", err)
	}
	setRequest := <-observed
	resetRequest := <-observed
	if setRequest.method != http.MethodPut || setRequest.path != "/api/v1/heartbeat/jobs/cloudbank-main/cron" || setRequest.profile != "idle" || setRequest.reason != "no actionable tasks" {
		t.Fatalf("unexpected set request: %#v", setRequest)
	}
	if resetRequest.method != http.MethodDelete || resetRequest.path != "/api/v1/heartbeat/jobs/cloudbank-main/cron" {
		t.Fatalf("unexpected reset request: %#v", resetRequest)
	}
}

func TestDownloadFileViaHTTPSlackAuth(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("slack-file-data"))
	}))
	defer server.Close()

	outputPath := filepath.Join(t.TempDir(), "downloads", "file.bin")
	cfg := &config.Config{
		Channels: &config.ChannelsConfig{
			Slack: &config.SlackConfig{BotToken: "xoxb-secret"},
		},
	}

	if err := downloadFileViaHTTP(context.Background(), cfg, "slack", server.URL+"/private/file", outputPath); err != nil {
		t.Fatalf("downloadFileViaHTTP: %v", err)
	}
	if gotAuth != "Bearer xoxb-secret" {
		t.Fatalf("authorization=%q", gotAuth)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "slack-file-data" {
		t.Fatalf("output data=%q", string(data))
	}
}

func TestDownloadFileViaHTTPTelegramNoAuthHeader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("telegram-file-data"))
	}))
	defer server.Close()

	outputPath := filepath.Join(t.TempDir(), "tg.bin")
	if err := downloadFileViaHTTP(context.Background(), &config.Config{}, "telegram", server.URL+"/bot123/file", outputPath); err != nil {
		t.Fatalf("downloadFileViaHTTP: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected empty Authorization header, got %q", gotAuth)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "telegram-file-data" {
		t.Fatalf("output data=%q", string(data))
	}
}

func TestDownloadFileViaHTTPSlackMissingToken(t *testing.T) {
	err := downloadFileViaHTTP(
		context.Background(),
		&config.Config{},
		"slack",
		"https://files.slack.com/files-pri/T/F",
		filepath.Join(t.TempDir(), "x.bin"),
	)
	if err == nil {
		t.Fatalf("expected missing token error")
	}
	if !strings.Contains(err.Error(), "channels.slack.botToken") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeMinimalConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configContents := []byte("gateway:\n  bind: 127.0.0.1\n  port: 18789\n")

	if err := os.WriteFile(configPath, configContents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return configPath
}

func TestShutdownSignalsIncludeSIGHUP(t *testing.T) {
	if !containsSignal(shutdownSignals, syscall.SIGHUP) {
		t.Fatalf("expected shutdown signals to include SIGHUP")
	}
}

func TestExitCodeForShutdown(t *testing.T) {
	if got := exitCodeForShutdown(0, syscall.SIGHUP, true); got != exitCodeRestartRequested {
		t.Fatalf("expected restart code %d, got %d", exitCodeRestartRequested, got)
	}
	if got := exitCodeForShutdown(0, syscall.SIGTERM, true); got != 0 {
		t.Fatalf("expected normal exit code for SIGTERM, got %d", got)
	}
	if got := exitCodeForShutdown(1, syscall.SIGHUP, true); got != 1 {
		t.Fatalf("expected existing failure code to be preserved, got %d", got)
	}
}

func containsSignal(signals []os.Signal, target os.Signal) bool {
	for _, sig := range signals {
		if sig == target {
			return true
		}
	}
	return false
}
