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
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/internal/gateway"
)

var messageSendFn = sendMessageViaGatewayAPI
var fileDownloadFn = downloadFileViaHTTP
var heartbeatCronSetFn = setHeartbeatCronViaGatewayAPI
var heartbeatCronResetFn = resetHeartbeatCronViaGatewayAPI

// stringSliceFlag implements flag.Value so a flag can be specified multiple
// times (e.g. --image a.png --image b.png) and accumulate into a slice.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *stringSliceFlag) trimmed() []string {
	if s == nil || len(*s) == 0 {
		return nil
	}
	out := make([]string, 0, len(*s))
	for _, value := range *s {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

const exitCodeRestartRequested = 75

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

func main() {
	os.Exit(Run(os.Args[1:], os.Stderr))
}

// Run executes the fractalbot CLI.
func Run(args []string, out io.Writer) int {
	ctx, stop, observedSignal := newShutdownContext(context.Background())
	code := runWithContext(ctx, args, out)
	stop()

	sig, ok := <-observedSignal
	return exitCodeForShutdown(code, sig, ok)
}

func exitCodeForShutdown(code int, sig os.Signal, hasSignal bool) int {
	if code == 0 && hasSignal && sig == syscall.SIGHUP {
		return exitCodeRestartRequested
	}
	return code
}

func newShutdownContext(parent context.Context) (context.Context, func(), <-chan os.Signal) {
	ctx, cancel := context.WithCancel(parent)
	sigCh := make(chan os.Signal, 1)
	observed := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals...)

	go func() {
		defer close(observed)
		select {
		case <-ctx.Done():
			return
		case <-parent.Done():
			cancel()
			return
		case sig := <-sigCh:
			observed <- sig
			cancel()
			return
		}
	}()

	cleanup := func() {
		signal.Stop(sigCh)
		cancel()
	}
	return ctx, cleanup, observed
}

func runWithContext(ctx context.Context, args []string, out io.Writer) int {
	fs := flag.NewFlagSet("fractalbot", flag.ContinueOnError)
	fs.SetOutput(out)

	configPath := fs.String("config", "", "path to config file (default: ~/.config/fractalbot/config.yaml)")
	portOverride := fs.Int("port", 0, "override gateway port")
	verbose := fs.Bool("verbose", false, "enable verbose logging")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	logger := log.New(out, "", log.LstdFlags)
	if *verbose {
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	cfg, err := config.LoadConfig(config.ResolveConfigPath(*configPath))
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
	case "file":
		return runFileCommand(ctx, cfg, args[1:], out, logger)
	case "heartbeat":
		return runHeartbeatCommand(ctx, cfg, args[1:], out, logger)
	default:
		logger.Printf("unknown command: %s", args[0])
		return 1
	}
}

func runHeartbeatCommand(ctx context.Context, cfg *config.Config, args []string, out io.Writer, logger *log.Logger) int {
	if len(args) < 2 || strings.ToLower(strings.TrimSpace(args[0])) != "cron" {
		logger.Printf("heartbeat command requires a cron subcommand (set or reset)")
		return 1
	}

	switch strings.ToLower(strings.TrimSpace(args[1])) {
	case "set":
		setFS := flag.NewFlagSet("heartbeat cron set", flag.ContinueOnError)
		setFS.SetOutput(out)
		job := setFS.String("job", "", "heartbeat job ID")
		profile := setFS.String("profile", "", "operator-approved cron profile")
		reason := setFS.String("reason", "", "reason for reducing heartbeat frequency")
		if err := setFS.Parse(args[2:]); err != nil {
			return 1
		}
		jobID := strings.TrimSpace(*job)
		profileName := strings.TrimSpace(*profile)
		reasonText := strings.TrimSpace(*reason)
		if jobID == "" {
			logger.Printf("--job is required")
			return 1
		}
		if profileName == "" {
			logger.Printf("--profile is required")
			return 1
		}
		if reasonText == "" {
			logger.Printf("--reason is required")
			return 1
		}
		if err := heartbeatCronSetFn(ctx, cfg, jobID, profileName, reasonText); err != nil {
			logger.Printf("failed to set heartbeat cron: %v", err)
			return 1
		}
		fmt.Fprintf(out, "Heartbeat job %s now uses cron profile %s\n", jobID, profileName)
		return 0

	case "reset":
		resetFS := flag.NewFlagSet("heartbeat cron reset", flag.ContinueOnError)
		resetFS.SetOutput(out)
		job := resetFS.String("job", "", "heartbeat job ID")
		if err := resetFS.Parse(args[2:]); err != nil {
			return 1
		}
		jobID := strings.TrimSpace(*job)
		if jobID == "" {
			logger.Printf("--job is required")
			return 1
		}
		if err := heartbeatCronResetFn(ctx, cfg, jobID); err != nil {
			logger.Printf("failed to reset heartbeat cron: %v", err)
			return 1
		}
		fmt.Fprintf(out, "Heartbeat job %s restored to its default cron\n", jobID)
		return 0

	default:
		logger.Printf("unknown heartbeat cron subcommand: %s", args[1])
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
	channel := sendFS.String("channel", "telegram", "target channel (e.g. telegram, slack, feishu, discord, imessage)")
	to := sendFS.String("to", "", "target chat ID")
	text := sendFS.String("text", "", "message text")
	threadTS := sendFS.String("thread-ts", "", "optional Slack thread timestamp for threaded reply")
	var imagePaths stringSliceFlag
	sendFS.Var(&imagePaths, "image", "local image path to attach (repeatable; issue #374)")

	if err := sendFS.Parse(args[1:]); err != nil {
		return 1
	}

	toValue := strings.TrimSpace(*to)
	if toValue == "" {
		logger.Printf("--to is required")
		return 1
	}

	messageText := strings.TrimSpace(*text)
	imageValues := imagePaths.trimmed()
	if messageText == "" && len(imageValues) == 0 {
		logger.Printf("--text or --image is required")
		return 1
	}

	channelName := strings.ToLower(strings.TrimSpace(*channel))
	if channelName == "" {
		logger.Printf("--channel is required")
		return 1
	}

	threadTSValue := strings.TrimSpace(*threadTS)

	if err := messageSendFn(ctx, cfg, channelName, toValue, messageText, threadTSValue, imageValues); err != nil {
		logger.Printf("failed to send message: %v", err)
		return 1
	}

	fmt.Fprintf(out, "✅ Message sent via %s to %s\n", channelName, toValue)
	return 0
}

func runFileCommand(ctx context.Context, cfg *config.Config, args []string, out io.Writer, logger *log.Logger) int {
	if len(args) == 0 {
		logger.Printf("file command requires a subcommand (download)")
		return 1
	}

	subcmd := strings.ToLower(strings.TrimSpace(args[0]))
	if subcmd != "download" {
		logger.Printf("unknown file subcommand: %s", args[0])
		return 1
	}

	downloadFS := flag.NewFlagSet("file download", flag.ContinueOnError)
	downloadFS.SetOutput(out)
	channel := downloadFS.String("channel", "", "source channel (slack or telegram)")
	fileURL := downloadFS.String("url", "", "file URL")
	output := downloadFS.String("output", "", "local output path")

	if err := downloadFS.Parse(args[1:]); err != nil {
		return 1
	}

	channelName := strings.ToLower(strings.TrimSpace(*channel))
	if channelName == "" {
		logger.Printf("--channel is required")
		return 1
	}
	urlValue := strings.TrimSpace(*fileURL)
	if urlValue == "" {
		logger.Printf("--url is required")
		return 1
	}
	outputPath := strings.TrimSpace(*output)
	if outputPath == "" {
		logger.Printf("--output is required")
		return 1
	}

	if err := fileDownloadFn(ctx, cfg, channelName, urlValue, outputPath); err != nil {
		logger.Printf("failed to download file: %v", err)
		return 1
	}

	fmt.Fprintf(out, "✅ File downloaded via %s to %s\n", channelName, outputPath)
	return 0
}

func sendMessageViaGatewayAPI(ctx context.Context, cfg *config.Config, channel string, to string, text string, threadTS string, images []string) error {
	type requestPayload struct {
		Channel  string   `json:"channel"`
		To       string   `json:"to"`
		Text     string   `json:"text"`
		ThreadTS string   `json:"thread_ts,omitempty"`
		Images   []string `json:"images,omitempty"`
	}

	type responsePayload struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}

	requestBody, err := json.Marshal(requestPayload{
		Channel:  channel,
		To:       to,
		Text:     text,
		ThreadTS: threadTS,
		Images:   images,
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
	return gatewayAPIEndpoint(cfg, "/api/v1/message/send")
}

func gatewayAPIEndpoint(cfg *config.Config, path string) string {
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

	return fmt.Sprintf("http://%s%s", net.JoinHostPort(bind, strconv.Itoa(port)), path)
}

func setHeartbeatCronViaGatewayAPI(ctx context.Context, cfg *config.Config, jobID, profile, reason string) error {
	payload, err := json.Marshal(heartbeatCronRequest{Profile: profile, Reason: reason})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	return callHeartbeatCronAPI(ctx, cfg, http.MethodPut, jobID, bytes.NewReader(payload))
}

func resetHeartbeatCronViaGatewayAPI(ctx context.Context, cfg *config.Config, jobID string) error {
	return callHeartbeatCronAPI(ctx, cfg, http.MethodDelete, jobID, nil)
}

type heartbeatCronRequest struct {
	Profile string `json:"profile"`
	Reason  string `json:"reason"`
}

func callHeartbeatCronAPI(ctx context.Context, cfg *config.Config, method, jobID string, body io.Reader) error {
	path := "/api/v1/heartbeat/jobs/" + url.PathEscape(strings.TrimSpace(jobID)) + "/cron"
	endpoint := gatewayAPIEndpoint(cfg, path)
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request %s failed: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		message := strings.TrimSpace(string(responseBody))
		var parsed struct {
			Error string `json:"error"`
		}
		if len(responseBody) > 0 && json.Unmarshal(responseBody, &parsed) == nil && strings.TrimSpace(parsed.Error) != "" {
			message = parsed.Error
		}
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("gateway API error (%d): %s", response.StatusCode, message)
	}
	return nil
}

func downloadFileViaHTTP(ctx context.Context, cfg *config.Config, channel, fileURL, outputPath string) error {
	channelName := strings.ToLower(strings.TrimSpace(channel))
	if channelName != "slack" && channelName != "telegram" {
		return fmt.Errorf("unsupported channel %q (expected slack or telegram)", channel)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(fileURL), nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	if channelName == "slack" {
		token, err := slackBotTokenFromConfig(cfg)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("download failed (%d): %s", response.StatusCode, message)
	}

	outputPath = strings.TrimSpace(outputPath)
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	tmpFile, err := os.CreateTemp(outputDir, ".fractalbot-download-*")
	if err != nil {
		return fmt.Errorf("create temp output file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmpFile, response.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write output file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return fmt.Errorf("finalize output file: %w", err)
	}
	return nil
}

func slackBotTokenFromConfig(cfg *config.Config) (string, error) {
	if cfg == nil || cfg.Channels == nil || cfg.Channels.Slack == nil {
		return "", fmt.Errorf("channels.slack.botToken is required for Slack file download")
	}
	token := strings.TrimSpace(cfg.Channels.Slack.BotToken)
	if token == "" {
		return "", fmt.Errorf("channels.slack.botToken is required for Slack file download")
	}
	return token, nil
}
