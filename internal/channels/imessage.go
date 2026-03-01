package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const (
	defaultIMessageService          = "E:iMessage"
	defaultIMessagePollingInterval  = 5 * time.Second
	minIMessagePollingInterval      = 1 * time.Second
	defaultIMessagePollingLimit     = 20
	maxIMessagePollingLimit         = 100
	defaultIMessageDatabasePath     = "~/Library/Messages/chat.db"
	defaultIMessageStartCheckPeriod = 500 * time.Millisecond
	defaultIMessageStartRetries     = 5
)

var currentGOOS = runtime.GOOS

type iMessageSQLiteRow struct {
	MessageID int64   `json:"message_id"`
	Sender    string  `json:"sender"`
	Text      string  `json:"text"`
	Date      float64 `json:"date"`
}

// IMessageInbound represents a single inbound iMessage entry.
type IMessageInbound struct {
	MessageID    int64
	Sender       string
	Text         string
	Timestamp    time.Time
	RawTimestamp int64
}

// IMessageBot implements native macOS iMessage send/polling.
type IMessageBot struct {
	recipient      string
	defaultMessage string
	service        string

	handler IncomingMessageHandler

	pollingEnabled  bool
	pollingInterval time.Duration
	pollingLimit    int
	databasePath    string

	lastSeenMu        sync.Mutex
	lastSeenMessageID int64

	execFn             func(ctx context.Context, name string, args ...string) ([]byte, error)
	readMessagesFn     func(ctx context.Context, sinceMessageID int64, limit int) ([]IMessageInbound, error)
	checkPermissionsFn func(ctx context.Context) error
	isMessagesRunning  func(ctx context.Context) (bool, error)
	startMessagesApp   func(ctx context.Context) error
	sleepFn            func(d time.Duration)

	runningMu sync.RWMutex
	running   bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	telemetryMu  sync.RWMutex
	lastActivity time.Time
	lastError    time.Time
}

func NewIMessageBot(recipient, defaultMessage, service string) (*IMessageBot, error) {
	if currentGOOS != "darwin" {
		return nil, errors.New("imessage channel is only supported on darwin")
	}

	trimmedRecipient := strings.TrimSpace(recipient)
	if trimmedRecipient == "" {
		return nil, errors.New("imessage recipient is required")
	}

	trimmedService := strings.TrimSpace(service)
	if trimmedService == "" {
		trimmedService = defaultIMessageService
	}

	bot := &IMessageBot{
		recipient:       trimmedRecipient,
		defaultMessage:  strings.TrimSpace(defaultMessage),
		service:         trimmedService,
		ctx:             context.Background(),
		pollingEnabled:  false,
		pollingInterval: defaultIMessagePollingInterval,
		pollingLimit:    defaultIMessagePollingLimit,
		databasePath:    defaultIMessageDatabasePath,
		sleepFn:         time.Sleep,
	}

	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
	bot.readMessagesFn = bot.readMessagesFromSQLite
	bot.checkPermissionsFn = bot.checkStartupPermissions
	bot.isMessagesRunning = bot.defaultIsMessagesRunning
	bot.startMessagesApp = bot.defaultStartMessagesApp

	return bot, nil
}

func (b *IMessageBot) Name() string {
	return "imessage"
}

func (b *IMessageBot) SetHandler(handler IncomingMessageHandler) {
	b.handler = handler
}

// ConfigurePolling configures inbound iMessage polling behavior.
func (b *IMessageBot) ConfigurePolling(enabled bool, intervalSeconds int, limit int, databasePath string) {
	b.pollingEnabled = enabled

	if intervalSeconds > 0 {
		b.pollingInterval = time.Duration(intervalSeconds) * time.Second
	} else {
		b.pollingInterval = defaultIMessagePollingInterval
	}
	if b.pollingInterval < minIMessagePollingInterval {
		b.pollingInterval = minIMessagePollingInterval
	}

	if limit > 0 {
		b.pollingLimit = limit
	} else {
		b.pollingLimit = defaultIMessagePollingLimit
	}
	if b.pollingLimit > maxIMessagePollingLimit {
		b.pollingLimit = maxIMessagePollingLimit
	}

	if trimmed := strings.TrimSpace(databasePath); trimmed != "" {
		b.databasePath = trimmed
	}
}

func (b *IMessageBot) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

func (b *IMessageBot) setRunning(running bool) {
	b.runningMu.Lock()
	b.running = running
	b.runningMu.Unlock()
}

// LastActivity reports the last successful send or inbound handling time.
func (b *IMessageBot) LastActivity() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastActivity
}

// LastError reports the last channel error time.
func (b *IMessageBot) LastError() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastError
}

func (b *IMessageBot) markActivity() {
	b.telemetryMu.Lock()
	b.lastActivity = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *IMessageBot) markError() {
	b.telemetryMu.Lock()
	b.lastError = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *IMessageBot) Start(ctx context.Context) error {
	if currentGOOS != "darwin" {
		return errors.New("imessage channel is only supported on darwin")
	}

	b.ctx, b.cancel = context.WithCancel(ctx)

	if b.pollingEnabled {
		if err := b.checkPermissionsFn(b.ctx); err != nil {
			b.markError()
			return err
		}
		if err := b.ensureMessagesRunning(b.ctx); err != nil {
			b.markError()
			return err
		}

		b.wg.Add(1)
		go b.pollLoop(b.ctx)
	}

	b.setRunning(true)
	return nil
}

func (b *IMessageBot) Stop() error {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.setRunning(false)
	return nil
}

func (b *IMessageBot) SendMessage(ctx context.Context, target string, text string) error {
	recipient := strings.TrimSpace(target)
	if recipient == "" {
		recipient = b.recipient
	}
	message := strings.TrimSpace(text)
	if message == "" {
		message = b.defaultMessage
	}
	return b.send(ctx, recipient, message)
}

// Send sends a message to the configured recipient using osascript.
func (b *IMessageBot) Send(text string) error {
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return b.send(ctx, b.recipient, text)
}

// PollMessages reads new inbound messages from Messages database.
func (b *IMessageBot) PollMessages(ctx context.Context) ([]IMessageInbound, error) {
	since := b.getLastSeenMessageID()
	msgs, err := b.readMessagesFn(ctx, since, b.pollingLimit)
	if err != nil {
		return nil, err
	}

	maxSeen := since
	for _, msg := range msgs {
		if msg.MessageID > maxSeen {
			maxSeen = msg.MessageID
		}
	}
	if maxSeen > since {
		b.setLastSeenMessageID(maxSeen)
	}

	return msgs, nil
}

func (b *IMessageBot) pollLoop(ctx context.Context) {
	defer b.wg.Done()

	b.pollOnce(ctx)
	ticker := time.NewTicker(b.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.pollOnce(ctx)
		}
	}
}

func (b *IMessageBot) pollOnce(ctx context.Context) {
	if b.handler == nil {
		return
	}

	msgs, err := b.PollMessages(ctx)
	if err != nil {
		b.markError()
		log.Printf("imessage polling failed: %v", err)
		return
	}

	for _, inbound := range msgs {
		if strings.TrimSpace(inbound.Text) == "" {
			continue
		}
		reply, err := b.handler.HandleIncoming(ctx, b.toProtocolMessage(inbound))
		if err != nil {
			b.markError()
			log.Printf("imessage inbound handler failed: %v", err)
			continue
		}

		b.markActivity()

		trimmedReply := strings.TrimSpace(reply)
		if trimmedReply != "" && strings.TrimSpace(inbound.Sender) != "" {
			if err := b.send(ctx, inbound.Sender, trimmedReply); err != nil {
				log.Printf("imessage reply send failed: %v", err)
			}
		}
	}
}

func (b *IMessageBot) toProtocolMessage(inbound IMessageInbound) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel":     "imessage",
			"text":        inbound.Text,
			"agent":       "",
			"sender":      inbound.Sender,
			"message_id":  inbound.MessageID,
			"timestamp":   inbound.Timestamp.UTC().Format(time.RFC3339),
			"chatType":    "dm",
			"raw_message": inbound.RawTimestamp,
		},
	}
}

func (b *IMessageBot) readMessagesFromSQLite(ctx context.Context, sinceMessageID int64, limit int) ([]IMessageInbound, error) {
	if b.execFn == nil {
		return nil, errors.New("imessage executor not configured")
	}
	if limit <= 0 {
		limit = defaultIMessagePollingLimit
	}
	if limit > maxIMessagePollingLimit {
		limit = maxIMessagePollingLimit
	}
	if sinceMessageID < 0 {
		sinceMessageID = 0
	}

	dbPath, err := expandHomePath(b.databasePath)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT
    m.ROWID AS message_id,
    COALESCE(h.id, '') AS sender,
    COALESCE(m.text, '') AS text,
    m.date AS date
FROM message m
LEFT JOIN handle h ON m.handle_id = h.ROWID
WHERE m.is_from_me = 0
  AND m.ROWID > %d
ORDER BY m.ROWID ASC
LIMIT %d;`, sinceMessageID, limit)

	output, execErr := b.execFn(ctx, "sqlite3", "-json", dbPath, query)
	if execErr != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput != "" {
			return nil, fmt.Errorf("imessage sqlite polling failed: %w: %s", execErr, trimmedOutput)
		}
		return nil, fmt.Errorf("imessage sqlite polling failed: %w", execErr)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var rows []iMessageSQLiteRow
	if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
		return nil, fmt.Errorf("parse sqlite messages: %w", err)
	}

	messages := make([]IMessageInbound, 0, len(rows))
	for _, row := range rows {
		text := strings.TrimSpace(row.Text)
		if text == "" {
			continue
		}
		rawTimestamp := int64(row.Date)
		messages = append(messages, IMessageInbound{
			MessageID:    row.MessageID,
			Sender:       strings.TrimSpace(row.Sender),
			Text:         text,
			Timestamp:    appleTimestampToTime(rawTimestamp),
			RawTimestamp: rawTimestamp,
		})
	}

	return messages, nil
}

func (b *IMessageBot) checkStartupPermissions(ctx context.Context) error {
	if b.execFn == nil {
		return errors.New("imessage executor not configured")
	}

	dbPath, err := expandHomePath(b.databasePath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("imessage database not accessible (%s): %w", dbPath, err)
	}

	output, err := b.execFn(ctx, "sqlite3", dbPath, "SELECT 1;")
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput != "" {
			return fmt.Errorf("imessage database permission check failed: %w: %s", err, trimmedOutput)
		}
		return fmt.Errorf("imessage database permission check failed: %w", err)
	}

	// Verify AppleScript access to Messages application.
	output, err = b.execFn(ctx, "osascript", "-e", `tell application "Messages" to get name`)
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput != "" {
			return fmt.Errorf("imessage app permission check failed: %w: %s", err, trimmedOutput)
		}
		return fmt.Errorf("imessage app permission check failed: %w", err)
	}

	return nil
}

func (b *IMessageBot) defaultIsMessagesRunning(ctx context.Context) (bool, error) {
	if b.execFn == nil {
		return false, errors.New("imessage executor not configured")
	}
	output, err := b.execFn(ctx, "osascript", "-e", `tell application "System Events" to (name of processes) contains "Messages"`)
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput != "" {
			return false, fmt.Errorf("check Messages process failed: %w: %s", err, trimmedOutput)
		}
		return false, fmt.Errorf("check Messages process failed: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(string(output)), "true"), nil
}

func (b *IMessageBot) defaultStartMessagesApp(ctx context.Context) error {
	if b.execFn == nil {
		return errors.New("imessage executor not configured")
	}
	if _, err := b.execFn(ctx, "open", "-a", "Messages"); err != nil {
		return fmt.Errorf("start Messages app: %w", err)
	}

	for i := 0; i < defaultIMessageStartRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		running, err := b.isMessagesRunning(ctx)
		if err == nil && running {
			return nil
		}
		b.sleepFn(defaultIMessageStartCheckPeriod)
	}

	return errors.New("messages app did not start in time")
}

func (b *IMessageBot) ensureMessagesRunning(ctx context.Context) error {
	running, err := b.isMessagesRunning(ctx)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	log.Printf("imessage: Messages app not running, attempting to start")
	return b.startMessagesApp(ctx)
}

func (b *IMessageBot) getLastSeenMessageID() int64 {
	b.lastSeenMu.Lock()
	defer b.lastSeenMu.Unlock()
	return b.lastSeenMessageID
}

func (b *IMessageBot) setLastSeenMessageID(value int64) {
	b.lastSeenMu.Lock()
	b.lastSeenMessageID = value
	b.lastSeenMu.Unlock()
}

func (b *IMessageBot) send(ctx context.Context, recipient, text string) error {
	if currentGOOS != "darwin" {
		return errors.New("imessage channel is only supported on darwin")
	}

	trimmedRecipient := strings.TrimSpace(recipient)
	if trimmedRecipient == "" {
		return errors.New("imessage recipient is required")
	}

	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" {
		return errors.New("imessage text is required")
	}

	if b.execFn == nil {
		return errors.New("imessage executor not configured")
	}

	script := buildIMessageScript(trimmedRecipient, trimmedText, b.service)
	output, err := b.execFn(ctx, "osascript", "-e", script)
	if err != nil {
		b.markError()
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput != "" {
			return fmt.Errorf("imessage osascript failed: %w: %s", err, trimmedOutput)
		}
		return fmt.Errorf("imessage osascript failed: %w", err)
	}

	b.markActivity()
	return nil
}

func buildIMessageScript(recipient, text, service string) string {
	return fmt.Sprintf(`tell application "Messages"
	send "%s" to buddy "%s" of service "%s"
end tell`, escapeAppleScriptString(text), escapeAppleScriptString(recipient), escapeAppleScriptString(service))
}

func escapeAppleScriptString(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", ``,
	)
	return replacer.Replace(value)
}

func expandHomePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("imessage database path is required")
	}
	if trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return home, nil
	}
	if strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(trimmed, "~/")), nil
	}
	return trimmed, nil
}

func appleTimestampToTime(raw int64) time.Time {
	if raw == 0 {
		return time.Time{}
	}
	base := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	// Apple Messages commonly stores nanoseconds since 2001-01-01.
	if raw > 9_000_000_000_000 {
		return base.Add(time.Duration(raw))
	}
	return base.Add(time.Duration(raw) * time.Second)
}
