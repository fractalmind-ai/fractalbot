package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/internal/gateway"
)

var messageSendFn = sendMessageViaGatewayAPI

func main() {
	os.Exit(Run(os.Args[1:], os.Stderr))
}

// Run executes the fractalbot CLI.
func Run(args []string, out io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runWithContext(ctx, args, out)
}

func runWithContext(ctx context.Context, args []string, out io.Writer) int {
	fs := flag.NewFlagSet("fractalbot", flag.ContinueOnError)
	fs.SetOutput(out)

	configPath := fs.String("config", "./config.yaml", "path to config file")
	portOverride := fs.Int("port", 0, "override gateway port")
	verbose := fs.Bool("verbose", false, "enable verbose logging")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	logger := log.New(out, "", log.LstdFlags)
	if *verbose {
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Printf("failed to load config: %v", err)
		return 1
	}

	remaining := fs.Args()
	if len(remaining) > 0 {
		return runCommand(ctx, cfg, remaining, out, logger)
	}

	if cfg.Gateway == nil {
		cfg.Gateway = &config.GatewayConfig{Bind: "127.0.0.1", Port: 18789}
	}
	if *portOverride > 0 {
		cfg.Gateway.Port = *portOverride
	}

	server, err := gateway.NewServer(cfg)
	if err != nil {
		logger.Printf("failed to initialize gateway: %v", err)
		return 1
	}

	if err := server.Start(ctx); err != nil {
		logger.Printf("gateway error: %v", err)
		if err := server.Stop(); err != nil {
			logger.Printf("gateway shutdown error: %v", err)
		}
		return 1
	}

	if err := server.Stop(); err != nil {
		logger.Printf("gateway shutdown error: %v", err)
		return 1
	}

	return 0
}

func runCommand(ctx context.Context, cfg *config.Config, args []string, out io.Writer, logger *log.Logger) int {
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "message":
		return runMessageCommand(ctx, cfg, args[1:], out, logger)
	default:
		logger.Printf("unknown command: %s", args[0])
		return 1
	}
}

func runMessageCommand(ctx context.Context, cfg *config.Config, args []string, out io.Writer, logger *log.Logger) int {
	if len(args) == 0 {
		logger.Printf("message command requires a subcommand (send)")
		return 1
	}

	subcmd := strings.ToLower(strings.TrimSpace(args[0]))
	if subcmd != "send" {
		logger.Printf("unknown message subcommand: %s", args[0])
		return 1
	}

	sendFS := flag.NewFlagSet("message send", flag.ContinueOnError)
	sendFS.SetOutput(out)
	channel := sendFS.String("channel", "telegram", "target channel (e.g. telegram, slack, feishu, discord)")
	to := sendFS.String("to", "", "target chat ID")
	text := sendFS.String("text", "", "message text")

	if err := sendFS.Parse(args[1:]); err != nil {
		return 1
	}

	toValue := strings.TrimSpace(*to)
	if toValue == "" {
		logger.Printf("--to is required")
		return 1
	}

	chatID, err := strconv.ParseInt(toValue, 10, 64)
	if err != nil {
		logger.Printf("invalid --to chat_id: %s", toValue)
		return 1
	}

	messageText := strings.TrimSpace(*text)
	if messageText == "" {
		logger.Printf("--text is required")
		return 1
	}

	channelName := strings.ToLower(strings.TrimSpace(*channel))
	if channelName == "" {
		logger.Printf("--channel is required")
		return 1
	}

	if err := messageSendFn(ctx, cfg, channelName, chatID, messageText); err != nil {
		logger.Printf("failed to send message: %v", err)
		return 1
	}

	fmt.Fprintf(out, "✅ Message sent via %s to %d\n", channelName, chatID)
	return 0
}

func sendMessageViaGatewayAPI(ctx context.Context, cfg *config.Config, channel string, to int64, text string) error {
	type requestPayload struct {
		Channel string `json:"channel"`
		To      int64  `json:"to"`
		Text    string `json:"text"`
	}

	type responsePayload struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}

	requestBody, err := json.Marshal(requestPayload{
		Channel: channel,
		To:      to,
		Text:    text,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	endpoint := gatewaySendEndpoint(cfg)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request %s failed: %w", endpoint, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		message := strings.TrimSpace(string(body))
		var parsed responsePayload
		if len(body) > 0 && json.Unmarshal(body, &parsed) == nil && strings.TrimSpace(parsed.Error) != "" {
			message = parsed.Error
		}
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("gateway API error (%d): %s", response.StatusCode, message)
	}

	return nil
}

func gatewaySendEndpoint(cfg *config.Config) string {
	bind := "127.0.0.1"
	port := 18789

	if cfg != nil && cfg.Gateway != nil {
		if trimmedBind := strings.TrimSpace(cfg.Gateway.Bind); trimmedBind != "" {
			bind = trimmedBind
		}
		if cfg.Gateway.Port > 0 {
			port = cfg.Gateway.Port
		}
	}

	if bind == "0.0.0.0" || bind == "::" {
		bind = "127.0.0.1"
	}

	return fmt.Sprintf("http://%s/api/v1/message/send", net.JoinHostPort(bind, strconv.Itoa(port)))
}
