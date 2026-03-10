package channels

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// imsgWatchEvent represents a JSON line from `imsg watch --json`.
type imsgWatchEvent struct {
	RowID       int64           `json:"rowid"`
	Text        string          `json:"text"`
	Sender      string          `json:"sender"`
	Timestamp   string          `json:"timestamp"`
	Attachments json.RawMessage `json:"attachments"`
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
	startWatchFn       func(ctx context.Context, recipient string) (io.ReadCloser, func() error, error)
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
	bot.startWatchFn = bot.defaultStartImsgWatch
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

		b.wg.Add(1)
		go b.watchLoop(b.ctx)
	}

	b.setRunning(true)
	return nil
}

func (b *IMessageBot) Stop(ctx context.Context) error {
	_ = ctx
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.setRunning(false)
	return nil
}

func (b *IMessageBot) Send(ctx context.Context, msg OutboundMessage) error {
	recipient := strings.TrimSpace(msg.To)
	if recipient == "" {
		recipient = b.recipient
	}
	message := strings.TrimSpace(msg.Text)
	if message == "" {
		message = b.defaultMessage
	}
	return b.send(ctx, recipient, message)
}

// IsAllowed always returns true for iMessage (single-recipient channel).
func (b *IMessageBot) IsAllowed(senderID string) bool {
	return true
}

func (b *IMessageBot) watchLoop(ctx context.Context) {
	defer b.wg.Done()

	for {
		startWatch := b.startWatchFn
		if startWatch == nil {
			startWatch = b.defaultStartImsgWatch
		}

		reader, waitFn, err := startWatch(ctx, b.recipient)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("imessage: failed to start imsg watch: %v", err)
			b.markError()
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		b.processImsgStream(ctx, reader)

		if waitFn != nil {
			_ = waitFn()
		}

		if ctx.Err() != nil {
			return
		}

		log.Printf("imessage: imsg watch exited, restarting...")
		b.markError()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (b *IMessageBot) processImsgStream(ctx context.Context, reader io.ReadCloser) {
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event imsgWatchEvent
		if err := json.Unmarshal(line, &event); err != nil {
			log.Printf("imessage: failed to parse imsg event: %v", err)
			continue
		}

		inbound := imsgEventToInbound(event)
		if strings.TrimSpace(inbound.Text) == "" {
			continue
		}

		if inbound.MessageID > 0 {
			b.setLastSeenMessageID(inbound.MessageID)
		}

		b.handleSingleInbound(ctx, inbound)
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		log.Printf("imessage: imsg stream read error: %v", err)
	}
}

func (b *IMessageBot) defaultStartImsgWatch(ctx context.Context, recipient string) (io.ReadCloser, func() error, error) {
	cmd := exec.CommandContext(ctx, "imsg", "watch", "--json", "--participants", recipient)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("imsg stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start imsg watch: %w", err)
	}
	return stdout, cmd.Wait, nil
}

func imsgEventToInbound(event imsgWatchEvent) IMessageInbound {
	ts, _ := time.Parse(time.RFC3339, event.Timestamp)
	return IMessageInbound{
		MessageID:   event.RowID,
		Sender:      strings.TrimSpace(event.Sender),
		Text:        strings.TrimSpace(event.Text),
		Timestamp:   ts,
		RawTimestamp: ts.UnixNano(),
	}
}

func (b *IMessageBot) handleSingleInbound(ctx context.Context, inbound IMessageInbound) {
	if b.handler == nil {
		return
	}

	reply, err := b.handler.HandleIncoming(ctx, b.toProtocolMessage(inbound))
	if err != nil {
		b.markError()
		log.Printf("imessage inbound handler failed: %v", err)
		return
	}

	b.markActivity()

	trimmedReply := strings.TrimSpace(reply)
	if trimmedReply != "" && strings.TrimSpace(inbound.Sender) != "" {
		if err := b.send(ctx, inbound.Sender, trimmedReply); err != nil {
			log.Printf("imessage reply send failed: %v", err)
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

func (b *IMessageBot) checkStartupPermissions(ctx context.Context) error {
	if b.execFn == nil {
		return errors.New("imessage executor not configured")
	}

	// Verify imsg CLI is available.
	output, err := b.execFn(ctx, "imsg", "--version")
	if err != nil {
		return fmt.Errorf("imsg CLI not found (install: brew install steipete/tap/imsg): %w", err)
	}
	log.Printf("imessage: imsg available: %s", strings.TrimSpace(string(output)))

	// Verify AppleScript access to Messages application (needed for sending).
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
