package channels

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
	_ "modernc.org/sqlite"
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
var iMessageURLRegexp = regexp.MustCompile(`https?://[^\s<>"'\\]+`)

type iMessageSQLiteRow struct {
	MessageID      int64   `json:"message_id"`
	Sender         string  `json:"sender"`
	Text           string  `json:"text"`
	AttributedBody []byte  `json:"attributed_body"`
	Date           float64 `json:"date"`
}

// IMessageInbound represents a single inbound iMessage entry.
type IMessageInbound struct {
	MessageID    int64
	Sender       string
	Text         string
	URLs         []string
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
	resolveServiceIDFn func(ctx context.Context) (string, error)
	sleepFn            func(d time.Duration)

	resolvedServiceID string

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
	bot.resolveServiceIDFn = bot.defaultResolveServiceID

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

	// Resolve iMessage service UUID for outbound sending (best-effort).
	serviceID, err := b.resolveServiceIDFn(b.ctx)
	if err != nil {
		log.Printf("imessage: failed to resolve service ID, outbound may fail: %v", err)
	} else {
		b.resolvedServiceID = serviceID
		log.Printf("imessage: resolved service ID: %s", serviceID)
	}

	if b.pollingEnabled {
		if err := b.ensureMessagesRunning(b.ctx); err != nil {
			b.markError()
			return err
		}
		if err := b.checkPermissionsFn(b.ctx); err != nil {
			b.markError()
			return err
		}
		maxID, err := b.getMaxMessageID(b.ctx)
		if err != nil {
			log.Printf("imessage: failed to get max message ID, starting from 0: %v", err)
		} else {
			b.setLastSeenMessageID(maxID)
			log.Printf("imessage: initialized lastSeenMessageID to %d", maxID)
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
	data := map[string]interface{}{
		"channel":     "imessage",
		"text":        inbound.Text,
		"agent":       "",
		"chat_id":     inbound.Sender,
		"user_id":     inbound.Sender,
		"sender":      inbound.Sender,
		"message_id":  inbound.MessageID,
		"timestamp":   inbound.Timestamp.UTC().Format(time.RFC3339),
		"chatType":    "dm",
		"raw_message": inbound.RawTimestamp,
	}
	if len(inbound.URLs) > 0 {
		data["urls"] = append([]string(nil), inbound.URLs...)
	}

	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data:   data,
	}
}

func (b *IMessageBot) readMessagesFromSQLite(ctx context.Context, sinceMessageID int64, limit int) ([]IMessageInbound, error) {
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

	// Open database using Go's database/sql with modernc.org/sqlite driver
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	defer db.Close()

	// Set busy timeout for concurrent access
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Query for new messages
	query := `SELECT
		m.ROWID AS message_id,
		COALESCE(h.id, '') AS sender,
		COALESCE(m.text, '') AS text,
		m.attributedBody AS attributed_body,
		m.date AS date
	FROM message m
	LEFT JOIN handle h ON m.handle_id = h.ROWID
	WHERE m.is_from_me = 0
	  AND m.ROWID > ?
	ORDER BY m.ROWID ASC
	LIMIT ?`

	rows, err := db.QueryContext(ctx, query, sinceMessageID, limit)
	if err != nil {
		return nil, fmt.Errorf("query sqlite: %w", err)
	}
	defer rows.Close()

	messages := []IMessageInbound{}
	for rows.Next() {
		var row iMessageSQLiteRow
		if err := rows.Scan(&row.MessageID, &row.Sender, &row.Text, &row.AttributedBody, &row.Date); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		text := strings.TrimSpace(row.Text)
		attributedText, attributedURLs := extractTextAndURLsFromAttributedBody(row.AttributedBody)
		if text == "" {
			text = attributedText
		} else if !containsURL(text) && len(attributedURLs) > 0 {
			text = mergeURLsIntoText(text, attributedURLs)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		rawTimestamp := int64(row.Date)
		messages = append(messages, IMessageInbound{
			MessageID:    row.MessageID,
			Sender:       strings.TrimSpace(row.Sender),
			Text:         text,
			URLs:         attributedURLs,
			Timestamp:    appleTimestampToTime(rawTimestamp),
			RawTimestamp: rawTimestamp,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return messages, nil
}

func (b *IMessageBot) getMaxMessageID(ctx context.Context) (int64, error) {
	dbPath, err := expandHomePath(b.databasePath)
	if err != nil {
		return 0, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, fmt.Errorf("open sqlite db: %w", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	var maxID int64
	err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(ROWID), 0) FROM message WHERE is_from_me = 0`).Scan(&maxID)
	if err != nil {
		return 0, fmt.Errorf("query max message id: %w", err)
	}

	if maxID < 0 {
		return 0, nil
	}
	return maxID, nil
}

func (b *IMessageBot) checkStartupPermissions(ctx context.Context) error {
	dbPath, err := expandHomePath(b.databasePath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("imessage database not accessible (%s): %w", dbPath, err)
	}

	// Try to open database with Go's sqlite driver to verify FDA
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite db: %w", err)
	}
	defer db.Close()

	// Set reasonable connection limits
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Try a simple query
	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("imessage database permission check failed: %w", err)
	}

	// Verify AppleScript access to Messages application.
	if b.execFn == nil {
		return errors.New("imessage executor not configured")
	}
	output, err := b.execFn(ctx, "osascript", "-e", `tell application "Messages" to get name`)
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

func (b *IMessageBot) defaultResolveServiceID(ctx context.Context) (string, error) {
	if b.execFn == nil {
		return "", errors.New("imessage executor not configured")
	}
	script := `tell application "Messages"
	set serviceList to every service
	repeat with s in serviceList
		try
			if (service type of s) as text is "iMessage" then
				return id of s as text
			end if
		end try
	end repeat
	return ""
end tell`
	output, err := b.execFn(ctx, "osascript", "-e", script)
	if err != nil {
		return "", fmt.Errorf("resolve iMessage service ID: %w", err)
	}
	serviceID := strings.TrimSpace(string(output))
	if serviceID == "" {
		return "", errors.New("no iMessage service found")
	}
	return serviceID, nil
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

	script := buildIMessageScript(trimmedRecipient, trimmedText, b.resolvedServiceID)
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

func buildIMessageScript(recipient, text, serviceID string) string {
	return fmt.Sprintf(`tell application "Messages"
	set targetService to service id "%s"
	set targetBuddy to buddy "%s" of targetService
	send "%s" to targetBuddy
end tell`, escapeAppleScriptString(serviceID), escapeAppleScriptString(recipient), escapeAppleScriptString(text))
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

func extractTextAndURLsFromAttributedBody(blob []byte) (string, []string) {
	urls := extractURLsFromAttributedBody(blob)
	text := extractReadableTextFromAttributedBody(blob)

	if len(urls) > 0 {
		if text == "" || looksLikeAttributedArchiveText(text) {
			text = strings.Join(urls, "\n")
		} else {
			text = mergeURLsIntoText(text, urls)
		}
	}

	return strings.TrimSpace(text), urls
}

func extractURLsFromAttributedBody(blob []byte) []string {
	if len(blob) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	urls := make([]string, 0, 2)
	collect := func(input []byte) {
		for _, match := range iMessageURLRegexp.FindAll(input, -1) {
			normalized := normalizeExtractedURL(string(match))
			if normalized == "" {
				continue
			}
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}
			urls = append(urls, normalized)
		}
	}

	collect(blob)
	if bytes.IndexByte(blob, 0x00) >= 0 {
		collect(bytes.ReplaceAll(blob, []byte{0x00}, nil))
	}

	return urls
}

func normalizeExtractedURL(url string) string {
	trimmed := strings.TrimSpace(url)
	trimmed = strings.TrimRight(trimmed, ".,;:!?)]}\"'")
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return ""
}

func extractReadableTextFromAttributedBody(blob []byte) string {
	if len(blob) == 0 {
		return ""
	}

	withoutNulls := bytes.ReplaceAll(blob, []byte{0x00}, nil)
	return normalizeAttributedText(string(withoutNulls))
}

func normalizeAttributedText(input string) string {
	if input == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(input))
	lastWasSpace := false
	for _, r := range input {
		switch {
		case unicode.IsPrint(r):
			builder.WriteRune(r)
			lastWasSpace = false
		case unicode.IsSpace(r):
			if !lastWasSpace {
				builder.WriteByte(' ')
				lastWasSpace = true
			}
		}
	}

	normalized := strings.Join(strings.Fields(builder.String()), " ")
	if len(normalized) < 3 {
		return ""
	}
	return strings.TrimSpace(normalized)
}

func containsURL(text string) bool {
	return iMessageURLRegexp.MatchString(text)
}

func mergeURLsIntoText(text string, urls []string) string {
	trimmed := strings.TrimSpace(text)
	if len(urls) == 0 {
		return trimmed
	}

	merged := trimmed
	for _, url := range urls {
		if containsURLWithValue(merged, url) {
			continue
		}
		if merged == "" {
			merged = url
		} else {
			merged += "\n" + url
		}
	}

	return strings.TrimSpace(merged)
}

func containsURLWithValue(text string, url string) bool {
	for _, match := range iMessageURLRegexp.FindAllString(text, -1) {
		if normalizeExtractedURL(match) == url {
			return true
		}
	}
	return false
}

func looksLikeAttributedArchiveText(text string) bool {
	lower := strings.ToLower(text)
	markers := []string{
		"streamtyped",
		"nskeyedarchiver",
		"nsdictionary",
		"nsstring",
		"__kimmessage",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
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
