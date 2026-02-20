package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to int64, text string) error {
			_ = ctx
			_ = cfg
			called = true
			if channel != "telegram" {
				t.Fatalf("channel=%q", channel)
			}
			if to != 5088760910 {
				t.Fatalf("to=%d", to)
			}
			if text != "hello from cli" {
				t.Fatalf("text=%q", text)
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
				name:          "invalid to",
				args:          []string{"--channel", "telegram", "--to", "not-number", "--text", "hello"},
				expectedError: "invalid --to chat_id",
			},
			{
				name:          "missing text",
				args:          []string{"--channel", "telegram", "--to", "5088760910"},
				expectedError: "--text is required",
			},
		}

		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to int64, text string) error {
					_ = ctx
					_ = cfg
					_ = channel
					_ = to
					_ = text
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
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to int64, text string) error {
			_ = ctx
			_ = cfg
			_ = channel
			_ = to
			_ = text
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
		messageSendFn = func(ctx context.Context, cfg *config.Config, channel string, to int64, text string) error {
			_ = ctx
			_ = cfg
			_ = channel
			_ = to
			_ = text
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
