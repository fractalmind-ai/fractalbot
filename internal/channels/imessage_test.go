package channels

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeIMessageHandler struct {
	reply string
	err   error
	calls int
	msgs  []*protocol.Message
}

func (f *fakeIMessageHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	_ = ctx
	f.calls++
	f.msgs = append(f.msgs, msg)
	return f.reply, f.err
}

type testIMessageRow struct {
	rowID    int64
	handleID int64
	sender   string
	isFromMe int
	text     string
	date     int64
}

func createTestIMessageDB(t *testing.T, rows []testIMessageRow) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "chat.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE handle (
		ROWID INTEGER PRIMARY KEY,
		id TEXT
	)`); err != nil {
		t.Fatalf("create handle table: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE message (
		ROWID INTEGER PRIMARY KEY,
		handle_id INTEGER,
		is_from_me INTEGER,
		text TEXT,
		date INTEGER
	)`); err != nil {
		t.Fatalf("create message table: %v", err)
	}

	for _, row := range rows {
		if row.handleID > 0 && strings.TrimSpace(row.sender) != "" {
			if _, err := db.Exec(`INSERT OR IGNORE INTO handle (ROWID, id) VALUES (?, ?)`, row.handleID, row.sender); err != nil {
				t.Fatalf("insert handle row: %v", err)
			}
		}
		if _, err := db.Exec(
			`INSERT INTO message (ROWID, handle_id, is_from_me, text, date) VALUES (?, ?, ?, ?, ?)`,
			row.rowID,
			row.handleID,
			row.isFromMe,
			row.text,
			row.date,
		); err != nil {
			t.Fatalf("insert message row: %v", err)
		}
	}

	return dbPath
}

func TestNewIMessageBotRejectsNonDarwin(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "linux"
	defer func() { currentGOOS = originalGOOS }()

	_, err := NewIMessageBot("recipient@example.com", "hello", "")
	if err == nil {
		t.Fatalf("expected non-darwin error")
	}
	if !strings.Contains(err.Error(), "darwin") {
		t.Fatalf("expected darwin error, got %v", err)
	}
}

func TestIMessageBotSendMessageUsesAppleScript(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "fallback", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	var called bool
	var commandName string
	var commandArgs []string
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		called = true
		commandName = name
		commandArgs = append([]string{}, args...)
		return []byte("ok"), nil
	}

	if err := bot.SendMessage(context.Background(), "", "hello from test"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if !called {
		t.Fatalf("expected osascript to be called")
	}
	if commandName != "osascript" {
		t.Fatalf("command=%q want osascript", commandName)
	}
	if len(commandArgs) != 2 || commandArgs[0] != "-e" {
		t.Fatalf("unexpected args: %v", commandArgs)
	}
	if !strings.Contains(commandArgs[1], `send "hello from test" to buddy "recipient@example.com" of service "E:iMessage"`) {
		t.Fatalf("unexpected script: %s", commandArgs[1])
	}
	if bot.LastActivity().IsZero() {
		t.Fatalf("expected last activity to be set")
	}
}

func TestIMessageBotSendMessageFallsBackToConfiguredMessage(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "configured default", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	var script string
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		_ = name
		if len(args) == 2 {
			script = args[1]
		}
		return nil, nil
	}

	if err := bot.SendMessage(context.Background(), "", ""); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.Contains(script, `send "configured default"`) {
		t.Fatalf("expected configured default message in script, got: %s", script)
	}
}

func TestIMessageBotPollMessagesFromSQLite(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	dbPath := createTestIMessageDB(t, []testIMessageRow{
		{rowID: 10, handleID: 1, sender: "+123", isFromMe: 1, text: "outbound", date: 785000000000000000},
		{rowID: 11, handleID: 1, sender: "+123", isFromMe: 0, text: "hello", date: 785000000000000001},
	})
	bot.ConfigurePolling(true, 5, 10, dbPath)

	msgs, err := bot.PollMessages(context.Background())
	if err != nil {
		t.Fatalf("PollMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs)=%d want 1", len(msgs))
	}
	if msgs[0].MessageID != 11 || msgs[0].Sender != "+123" || msgs[0].Text != "hello" {
		t.Fatalf("unexpected message: %+v", msgs[0])
	}
	if bot.getLastSeenMessageID() != 11 {
		t.Fatalf("lastSeenMessageID=%d want 11", bot.getLastSeenMessageID())
	}
}

func TestIMessageBotGetMaxMessageID(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	dbPath := createTestIMessageDB(t, []testIMessageRow{
		{rowID: 2, handleID: 1, sender: "+111", isFromMe: 0, text: "a", date: 1},
		{rowID: 5, handleID: 1, sender: "+111", isFromMe: 1, text: "b", date: 2},
		{rowID: 9, handleID: 2, sender: "+222", isFromMe: 0, text: "c", date: 3},
	})
	bot.ConfigurePolling(true, 5, 10, dbPath)

	maxID, err := bot.getMaxMessageID(context.Background())
	if err != nil {
		t.Fatalf("getMaxMessageID: %v", err)
	}
	if maxID != 9 {
		t.Fatalf("maxID=%d want 9", maxID)
	}
}

func TestIMessageBotPollOnceRoutesInboundAndReplies(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	handler := &fakeIMessageHandler{reply: "ack"}
	bot.SetHandler(handler)

	bot.readMessagesFn = func(ctx context.Context, sinceMessageID int64, limit int) ([]IMessageInbound, error) {
		_ = ctx
		_ = sinceMessageID
		_ = limit
		return []IMessageInbound{
			{
				MessageID: 1,
				Sender:    "+123",
				Text:      "ping",
				Timestamp: time.Now().UTC(),
			},
		}, nil
	}

	var replySent bool
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		if name == "osascript" && len(args) == 2 && strings.Contains(args[1], `send "ack" to buddy "+123"`) {
			replySent = true
		}
		return nil, nil
	}

	bot.pollOnce(context.Background())

	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1", handler.calls)
	}
	if !replySent {
		t.Fatalf("expected reply to be sent to inbound sender")
	}
	if bot.LastActivity().IsZero() {
		t.Fatalf("expected last activity to be set")
	}
}

func TestIMessageBotStartChecksPermissionsAndStartsMessages(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	dbPath := createTestIMessageDB(t, []testIMessageRow{
		{rowID: 3, handleID: 1, sender: "+123", isFromMe: 0, text: "hello", date: 1},
	})
	bot.ConfigurePolling(true, 60, 10, dbPath)

	var permissionChecked bool
	bot.checkPermissionsFn = func(ctx context.Context) error {
		_ = ctx
		permissionChecked = true
		return nil
	}

	running := false
	bot.isMessagesRunning = func(ctx context.Context) (bool, error) {
		_ = ctx
		return running, nil
	}

	var startCalled bool
	bot.startMessagesApp = func(ctx context.Context) error {
		_ = ctx
		startCalled = true
		running = true
		return nil
	}

	if err := bot.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !permissionChecked {
		t.Fatalf("expected permission check")
	}
	if !startCalled {
		t.Fatalf("expected Messages app start")
	}
	if err := bot.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestIMessageBotStartInitializesLastSeenMessageID(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	dbPath := createTestIMessageDB(t, []testIMessageRow{
		{rowID: 41, handleID: 1, sender: "+123", isFromMe: 0, text: "a", date: 1},
		{rowID: 42, handleID: 1, sender: "+123", isFromMe: 1, text: "b", date: 2},
		{rowID: 43, handleID: 2, sender: "+456", isFromMe: 0, text: "c", date: 3},
	})
	bot.ConfigurePolling(true, 60, 10, dbPath)

	bot.checkPermissionsFn = func(ctx context.Context) error {
		_ = ctx
		return nil
	}
	bot.isMessagesRunning = func(ctx context.Context) (bool, error) {
		_ = ctx
		return true, nil
	}

	if err := bot.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := bot.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}()

	if got := bot.getLastSeenMessageID(); got != 43 {
		t.Fatalf("lastSeenMessageID=%d want 43", got)
	}
}

func TestIMessageBotStartFailsPermissionCheck(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	bot.ConfigurePolling(true, 60, 10, "/tmp/chat.db")
	bot.checkPermissionsFn = func(ctx context.Context) error {
		_ = ctx
		return errors.New("permission denied")
	}

	err = bot.Start(context.Background())
	if err == nil {
		t.Fatalf("expected start error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("unexpected error: %v", err)
	}
}
