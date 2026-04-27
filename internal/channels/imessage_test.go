package channels

import (
	"context"
	"errors"
	"io"
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
	bot.resolvedServiceID = "TEST-UUID-123"

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

	if _, err := bot.Send(context.Background(), OutboundMessage{Text: "hello from test"}); err != nil {
		t.Fatalf("Send: %v", err)
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
	if !strings.Contains(commandArgs[1], `send "hello from test" to targetBuddy`) {
		t.Fatalf("unexpected script: %s", commandArgs[1])
	}
	if !strings.Contains(commandArgs[1], `service id "TEST-UUID-123"`) {
		t.Fatalf("expected service id in script: %s", commandArgs[1])
	}
	if bot.LastActivity().IsZero() {
		t.Fatalf("expected last activity to be set")
	}
}

func TestIMessageBotSendFallsBackToConfiguredMessage(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "configured default", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	bot.resolvedServiceID = "TEST-UUID"

	var script string
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		_ = name
		if len(args) == 2 {
			script = args[1]
		}
		return nil, nil
	}

	if _, err := bot.Send(context.Background(), OutboundMessage{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(script, `send "configured default"`) {
		t.Fatalf("expected configured default message in script, got: %s", script)
	}
}

func TestImsgWatchStreamProcessing(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+123", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	handler := &fakeIMessageHandler{reply: "got it"}
	bot.SetHandler(handler)
	bot.resolvedServiceID = "TEST-UUID"

	var replySenders []string
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		if name == "osascript" && len(args) == 2 {
			replySenders = append(replySenders, args[1])
		}
		return nil, nil
	}

	events := strings.Join([]string{
		`{"rowid":1,"text":"hello","sender":"+999","timestamp":"2026-03-04T13:00:00Z","attachments":[]}`,
		`{"rowid":2,"text":"world","sender":"+888","timestamp":"2026-03-04T13:01:00Z","attachments":[]}`,
	}, "\n")

	reader := io.NopCloser(strings.NewReader(events))
	bot.processImsgStream(context.Background(), reader)

	if handler.calls != 2 {
		t.Fatalf("handler calls=%d want 2", handler.calls)
	}
	data0 := handler.msgs[0].Data.(map[string]interface{})
	if data0["text"] != "hello" {
		t.Fatalf("first msg text=%q want hello", data0["text"])
	}
	if data0["sender"] != "+999" {
		t.Fatalf("first msg sender=%q want +999", data0["sender"])
	}
	data1 := handler.msgs[1].Data.(map[string]interface{})
	if data1["text"] != "world" {
		t.Fatalf("second msg text=%q want world", data1["text"])
	}
	if len(replySenders) != 2 {
		t.Fatalf("expected 2 replies, got %d", len(replySenders))
	}
	if bot.getLastSeenMessageID() != 2 {
		t.Fatalf("lastSeenMessageID=%d want 2", bot.getLastSeenMessageID())
	}
}

func TestImsgWatchStreamSkipsEmptyText(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+123", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	handler := &fakeIMessageHandler{reply: "ok"}
	bot.SetHandler(handler)
	bot.resolvedServiceID = "TEST-UUID"
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, nil
	}

	events := strings.Join([]string{
		`{"rowid":1,"text":"","sender":"+999","timestamp":"2026-03-04T13:00:00Z","attachments":[]}`,
		`{"rowid":2,"text":"  ","sender":"+999","timestamp":"2026-03-04T13:00:01Z","attachments":[]}`,
		`{"rowid":3,"text":"real message","sender":"+999","timestamp":"2026-03-04T13:00:02Z","attachments":[]}`,
	}, "\n")

	reader := io.NopCloser(strings.NewReader(events))
	bot.processImsgStream(context.Background(), reader)

	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1 (empty text should be skipped)", handler.calls)
	}
	data := handler.msgs[0].Data.(map[string]interface{})
	if data["text"] != "real message" {
		t.Fatalf("msg text=%q want 'real message'", data["text"])
	}
}

func TestImsgWatchStreamHandlesInvalidJSON(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+123", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	handler := &fakeIMessageHandler{reply: "ok"}
	bot.SetHandler(handler)
	bot.resolvedServiceID = "TEST-UUID"
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, nil
	}

	events := strings.Join([]string{
		`not-json-at-all`,
		`{"rowid":1,"text":"valid","sender":"+999","timestamp":"2026-03-04T13:00:00Z","attachments":[]}`,
		`{"broken json`,
		`{"rowid":2,"text":"also valid","sender":"+888","timestamp":"2026-03-04T13:01:00Z","attachments":[]}`,
	}, "\n")

	reader := io.NopCloser(strings.NewReader(events))
	bot.processImsgStream(context.Background(), reader)

	if handler.calls != 2 {
		t.Fatalf("handler calls=%d want 2 (invalid JSON should be skipped)", handler.calls)
	}
}

func TestImsgEventToInbound(t *testing.T) {
	event := imsgWatchEvent{
		RowID:     42,
		Text:      "  hello world  ",
		Sender:    " +123 ",
		Timestamp: "2026-03-04T13:00:00Z",
	}

	inbound := imsgEventToInbound(event)

	if inbound.MessageID != 42 {
		t.Fatalf("MessageID=%d want 42", inbound.MessageID)
	}
	if inbound.Text != "hello world" {
		t.Fatalf("Text=%q want 'hello world'", inbound.Text)
	}
	if inbound.Sender != "+123" {
		t.Fatalf("Sender=%q want '+123'", inbound.Sender)
	}
	expected := time.Date(2026, 3, 4, 13, 0, 0, 0, time.UTC)
	if !inbound.Timestamp.Equal(expected) {
		t.Fatalf("Timestamp=%v want %v", inbound.Timestamp, expected)
	}
}

func TestBuildImsgWatchArgsUsesSinceRowIDWhenAvailable(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	bot.watchStartTime = time.Date(2026, 3, 22, 18, 15, 59, 0, time.UTC)
	bot.setLastSeenMessageID(635)

	args := bot.buildImsgWatchArgs("+8619575545051")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "watch --json --participants +8619575545051 --since-rowid 635") {
		t.Fatalf("unexpected args: %v", args)
	}
	if strings.Contains(got, "--start") {
		t.Fatalf("expected --start to be omitted when since-rowid is available: %v", args)
	}
}

func TestBuildImsgWatchArgsFallsBackToStartTime(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	bot.watchStartTime = time.Date(2026, 3, 22, 18, 15, 59, 0, time.UTC)

	args := bot.buildImsgWatchArgs("+8619575545051")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "watch --json --participants +8619575545051 --start 2026-03-22T18:15:59Z") {
		t.Fatalf("unexpected args: %v", args)
	}
	if strings.Contains(got, "--since-rowid") {
		t.Fatalf("expected --since-rowid to be omitted without baseline: %v", args)
	}
}

func TestImsgWatchReconnectsOnProcessExit(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+123", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	handler := &fakeIMessageHandler{reply: ""}
	bot.SetHandler(handler)
	bot.resolvedServiceID = "TEST-UUID"
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, nil
	}

	startCount := 0
	bot.startWatchFn = func(ctx context.Context, recipient string) (io.ReadCloser, func() error, error) {
		startCount++
		if startCount == 1 {
			// First call: return a single message then EOF (simulates process exit)
			bot.watchStartTime = time.Time{}
			events := `{"rowid":1,"text":"first","sender":"+999","timestamp":"2026-03-04T13:00:00Z","attachments":[]}`
			return io.NopCloser(strings.NewReader(events)), func() error { return nil }, nil
		}
		// Second call: block until context cancelled (simulates stable process)
		pr, pw := io.Pipe()
		go func() {
			<-ctx.Done()
			pw.Close()
		}()
		return pr, func() error { return nil }, nil
	}

	bot.ConfigurePolling(true, 60, 10, "")
	bot.checkPermissionsFn = func(ctx context.Context) error { return nil }
	bot.isMessagesRunning = func(ctx context.Context) (bool, error) { return true, nil }
	bot.resolveServiceIDFn = func(ctx context.Context) (string, error) { return "TEST-UUID", nil }

	if err := bot.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the first message to be processed and reconnection to happen
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if handler.calls >= 1 && startCount >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := bot.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if handler.calls < 1 {
		t.Fatalf("handler calls=%d want >= 1", handler.calls)
	}
	if startCount < 2 {
		t.Fatalf("startCount=%d want >= 2 (reconnect)", startCount)
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
	bot.ConfigurePolling(true, 60, 10, "")

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
	bot.resolveServiceIDFn = func(ctx context.Context) (string, error) {
		return "TEST-UUID", nil
	}

	// Mock imsg watch: block until context cancelled
	bot.startWatchFn = func(ctx context.Context, recipient string) (io.ReadCloser, func() error, error) {
		pr, pw := io.Pipe()
		go func() {
			<-ctx.Done()
			pw.Close()
		}()
		return pr, func() error { return nil }, nil
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
	if err := bot.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
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
	bot.ConfigurePolling(true, 60, 10, "")
	bot.isMessagesRunning = func(ctx context.Context) (bool, error) {
		_ = ctx
		return true, nil
	}
	bot.resolveServiceIDFn = func(ctx context.Context) (string, error) {
		return "TEST-UUID", nil
	}
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

func TestCheckStartupPermissionsVerifiesImsgCLI(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	var cmds []string
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name
		if len(args) > 0 {
			cmd += " " + strings.Join(args, " ")
		}
		cmds = append(cmds, cmd)
		if name == "imsg" {
			return []byte("imsg v1.0.0"), nil
		}
		return []byte("Messages"), nil
	}

	if err := bot.checkStartupPermissions(context.Background()); err != nil {
		t.Fatalf("checkStartupPermissions: %v", err)
	}

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[0], "imsg --version") {
		t.Fatalf("first command should be imsg --version, got %q", cmds[0])
	}
	if !strings.Contains(cmds[1], "osascript") {
		t.Fatalf("second command should be osascript, got %q", cmds[1])
	}
}

func TestCheckStartupPermissionsFailsWithoutImsg(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("recipient@example.com", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "imsg" {
			return nil, errors.New("not found")
		}
		return nil, nil
	}

	err = bot.checkStartupPermissions(context.Background())
	if err == nil {
		t.Fatalf("expected error when imsg not available")
	}
	if !strings.Contains(err.Error(), "imsg CLI not found") {
		t.Fatalf("expected imsg not found error, got: %v", err)
	}
}

func TestHandleSingleInboundDispatchesAndReplies(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+123", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	handler := &fakeIMessageHandler{reply: "ack"}
	bot.SetHandler(handler)
	bot.resolvedServiceID = "TEST-UUID"

	var replySent bool
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		if name == "osascript" && len(args) == 2 && strings.Contains(args[1], `send "ack" to targetBuddy`) {
			replySent = true
		}
		return nil, nil
	}

	inbound := IMessageInbound{
		MessageID: 1,
		Sender:    "+999",
		Text:      "ping",
		Timestamp: time.Now().UTC(),
	}

	bot.handleSingleInbound(context.Background(), inbound)

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

func TestParseImsgChats(t *testing.T) {
	output := []byte(strings.Join([]string{
		`{"name":"","last_message_at":"2026-03-22T18:04:26.590Z","identifier":"+8619575545051","id":3,"service":"iMessage"}`,
		`{"name":"","last_message_at":"2026-02-01T14:12:28.893Z","identifier":"yubing744@gmail.com","id":2,"service":"iMessage"}`,
	}, "\n"))

	chats, err := parseImsgChats(output)
	if err != nil {
		t.Fatalf("parseImsgChats: %v", err)
	}
	if len(chats) != 2 {
		t.Fatalf("len(chats)=%d want 2", len(chats))
	}
	if chats[0].ID != 3 {
		t.Fatalf("chats[0].ID=%d want 3", chats[0].ID)
	}
	if chats[0].Identifier != "+8619575545051" {
		t.Fatalf("chats[0].Identifier=%q want +8619575545051", chats[0].Identifier)
	}
}

func TestDefaultResolveChatID(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		if name != "imsg" {
			t.Fatalf("unexpected command %q", name)
		}
		if len(args) != 2 || args[0] != "chats" || args[1] != "--json" {
			t.Fatalf("unexpected args: %v", args)
		}
		return []byte(strings.Join([]string{
			`{"identifier":"+100","id":1,"service":"iMessage"}`,
			`{"identifier":"+8619575545051","id":3,"service":"iMessage"}`,
		}, "\n")), nil
	}

	chatID, err := bot.defaultResolveChatID(context.Background(), "+8619575545051")
	if err != nil {
		t.Fatalf("defaultResolveChatID: %v", err)
	}
	if chatID != 3 {
		t.Fatalf("chatID=%d want 3", chatID)
	}
}

func TestParseImsgHistory(t *testing.T) {
	output := []byte(strings.Join([]string{
		`{"created_at":"2026-03-22T18:04:26.590Z","chat_id":3,"sender":"+8619575545051","text":"汇报一下OKR进度","id":635,"guid":"FF15563D-6BD5-4691-99FC-B2EFD4C44433","is_from_me":false,"attachments":[],"reactions":[]}`,
		`{"id":634,"created_at":"2026-03-22T18:01:44.618Z","reactions":[],"sender":"+8619575545051","guid":"C68BF610-041D-4128-88F8-EDAAD93A41C5","chat_id":3,"attachments":[],"text":"FractalBot fully up retest","is_from_me":true}`,
	}, "\n"))

	events, err := parseImsgHistory(output)
	if err != nil {
		t.Fatalf("parseImsgHistory: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events)=%d want 2", len(events))
	}
	if events[0].ID != 635 {
		t.Fatalf("events[0].ID=%d want 635", events[0].ID)
	}
	if events[0].Text != "汇报一下OKR进度" {
		t.Fatalf("events[0].Text=%q want 汇报一下OKR进度", events[0].Text)
	}
	if events[0].IsFromMe {
		t.Fatalf("events[0].IsFromMe=true want false")
	}
}

func TestInitializeLastSeenMessageIDUsesLatestHistoryEvent(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	bot.resolveChatIDFn = func(ctx context.Context, recipient string) (int64, error) {
		_ = ctx
		if recipient != "+8619575545051" {
			t.Fatalf("recipient=%q want +8619575545051", recipient)
		}
		return 3, nil
	}
	bot.fetchHistoryFn = func(ctx context.Context, chatID int64, limit int) ([]imsgHistoryEvent, error) {
		_ = ctx
		if chatID != 3 {
			t.Fatalf("chatID=%d want 3", chatID)
		}
		if limit != 1 {
			t.Fatalf("limit=%d want 1", limit)
		}
		return []imsgHistoryEvent{
			{ID: 635, IsFromMe: false, Text: "汇报一下OKR进度", Sender: "+8619575545051", CreatedAt: "2026-03-22T18:04:26.590Z"},
		}, nil
	}

	if err := bot.initializeLastSeenMessageID(context.Background()); err != nil {
		t.Fatalf("initializeLastSeenMessageID: %v", err)
	}
	if bot.getLastSeenMessageID() != 635 {
		t.Fatalf("lastSeenMessageID=%d want 635", bot.getLastSeenMessageID())
	}
}

func TestPollOnceProcessesOnlyUnseenInboundHistory(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	bot.pollingLimit = 20
	bot.setLastSeenMessageID(634)

	handler := &fakeIMessageHandler{}
	bot.SetHandler(handler)
	bot.resolveChatIDFn = func(ctx context.Context, recipient string) (int64, error) {
		_ = ctx
		_ = recipient
		return 3, nil
	}
	bot.fetchHistoryFn = func(ctx context.Context, chatID int64, limit int) ([]imsgHistoryEvent, error) {
		_ = ctx
		if chatID != 3 {
			t.Fatalf("chatID=%d want 3", chatID)
		}
		if limit != 20 {
			t.Fatalf("limit=%d want 20", limit)
		}
		return []imsgHistoryEvent{
			{ID: 636, IsFromMe: true, Text: "reply", Sender: "+8619575545051", CreatedAt: "2026-03-22T18:05:26.590Z"},
			{ID: 635, IsFromMe: false, Text: "汇报一下OKR进度", Sender: "+8619575545051", CreatedAt: "2026-03-22T18:04:26.590Z"},
			{ID: 634, IsFromMe: true, Text: "FractalBot fully up retest", Sender: "+8619575545051", CreatedAt: "2026-03-22T18:01:44.618Z"},
		}, nil
	}

	if err := bot.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1", handler.calls)
	}
	data := handler.msgs[0].Data.(map[string]interface{})
	if data["text"] != "汇报一下OKR进度" {
		t.Fatalf("text=%q want 汇报一下OKR进度", data["text"])
	}
	if bot.getLastSeenMessageID() != 635 {
		t.Fatalf("lastSeenMessageID=%d want 635", bot.getLastSeenMessageID())
	}

	if err := bot.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce second call: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1 after dedupe", handler.calls)
	}
}

func TestWatchAndPollDeduplicateByMessageID(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}

	handler := &fakeIMessageHandler{}
	bot.SetHandler(handler)
	bot.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, nil
	}
	bot.resolveChatIDFn = func(ctx context.Context, recipient string) (int64, error) {
		_ = ctx
		_ = recipient
		return 3, nil
	}
	bot.fetchHistoryFn = func(ctx context.Context, chatID int64, limit int) ([]imsgHistoryEvent, error) {
		_ = ctx
		_ = limit
		if chatID != 3 {
			t.Fatalf("chatID=%d want 3", chatID)
		}
		return []imsgHistoryEvent{
			{ID: 635, IsFromMe: false, Text: "same message", Sender: "+8619575545051", CreatedAt: "2026-03-22T18:04:26.590Z"},
		}, nil
	}

	reader := io.NopCloser(strings.NewReader(`{"rowid":635,"text":"same message","sender":"+8619575545051","timestamp":"2026-03-22T18:04:26.590Z","attachments":[]}`))
	bot.processImsgStream(context.Background(), reader)

	if err := bot.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1", handler.calls)
	}
}

func TestImsgWatchDropsHistoricalMessagesBeforeStartWhenNoBaseline(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	bot.watchStartTime = time.Date(2026, 3, 22, 18, 27, 10, 0, time.UTC)

	handler := &fakeIMessageHandler{}
	bot.SetHandler(handler)

	reader := io.NopCloser(strings.NewReader(strings.Join([]string{
		`{"rowid":635,"text":"old message","sender":"+8619575545051","timestamp":"2026-03-22T18:23:59Z","attachments":[]}`,
		`{"rowid":636,"text":"new message","sender":"+8619575545051","timestamp":"2026-03-22T18:27:11Z","attachments":[]}`,
	}, "\n")))

	bot.processImsgStream(context.Background(), reader)

	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1", handler.calls)
	}
	data := handler.msgs[0].Data.(map[string]interface{})
	if data["text"] != "new message" {
		t.Fatalf("text=%q want new message", data["text"])
	}
	if bot.getLastSeenMessageID() != 636 {
		t.Fatalf("lastSeenMessageID=%d want 636", bot.getLastSeenMessageID())
	}
}

func TestPollOnceDropsHistoricalMessagesBeforeStartWhenNoBaseline(t *testing.T) {
	originalGOOS := currentGOOS
	currentGOOS = "darwin"
	defer func() { currentGOOS = originalGOOS }()

	bot, err := NewIMessageBot("+8619575545051", "", "")
	if err != nil {
		t.Fatalf("NewIMessageBot: %v", err)
	}
	bot.watchStartTime = time.Date(2026, 3, 22, 18, 27, 10, 0, time.UTC)
	bot.pollingLimit = 20

	handler := &fakeIMessageHandler{}
	bot.SetHandler(handler)
	bot.resolveChatIDFn = func(ctx context.Context, recipient string) (int64, error) {
		_ = ctx
		_ = recipient
		return 3, nil
	}
	bot.fetchHistoryFn = func(ctx context.Context, chatID int64, limit int) ([]imsgHistoryEvent, error) {
		_ = ctx
		_ = chatID
		_ = limit
		return []imsgHistoryEvent{
			{ID: 635, IsFromMe: false, Text: "old message", Sender: "+8619575545051", CreatedAt: "2026-03-22T18:23:59Z"},
			{ID: 636, IsFromMe: false, Text: "new message", Sender: "+8619575545051", CreatedAt: "2026-03-22T18:27:11Z"},
		}, nil
	}

	if err := bot.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls=%d want 1", handler.calls)
	}
	data := handler.msgs[0].Data.(map[string]interface{})
	if data["text"] != "new message" {
		t.Fatalf("text=%q want new message", data["text"])
	}
	if bot.getLastSeenMessageID() != 636 {
		t.Fatalf("lastSeenMessageID=%d want 636", bot.getLastSeenMessageID())
	}
}
