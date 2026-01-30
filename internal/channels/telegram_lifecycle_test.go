package channels

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeLifecycle struct {
	monitorCalled bool
	monitorAgent  string
	monitorLines  int
	monitorErr    error

	startCalled bool
	startAgent  string
	startErr    error

	stopCalled bool
	stopAgent  string
	stopErr    error

	doctorCalled bool
	doctorErr    error
}

func (f *fakeLifecycle) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	return "", nil
}

func (f *fakeLifecycle) MonitorAgent(ctx context.Context, agentName string, lines int) (string, error) {
	f.monitorCalled = true
	f.monitorAgent = agentName
	f.monitorLines = lines
	return "ok", f.monitorErr
}

func (f *fakeLifecycle) StartAgent(ctx context.Context, agentName string) (string, error) {
	f.startCalled = true
	f.startAgent = agentName
	return "", f.startErr
}

func (f *fakeLifecycle) StopAgent(ctx context.Context, agentName string) (string, error) {
	f.stopCalled = true
	f.stopAgent = agentName
	return "", f.stopErr
}

func (f *fakeLifecycle) Doctor(ctx context.Context) (string, error) {
	f.doctorCalled = true
	return "", f.doctorErr
}

type fakeHandler struct{}

func (f *fakeHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	return "", nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func stubHTTPClient() *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
}

func TestTelegramMonitorDispatch(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}
	bot.httpClient = stubHTTPClient()

	lifecycle := &fakeLifecycle{}
	bot.SetHandler(lifecycle)

	msg := &TelegramMessage{
		Text: "/monitor qa-1 50",
		From: &TelegramUser{ID: 111},
		Chat: &TelegramChat{ID: 1},
	}

	handled, err := bot.handleCommand(msg)
	if !handled {
		t.Fatalf("expected handled")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lifecycle.monitorCalled {
		t.Fatalf("expected MonitorAgent to be called")
	}
	if lifecycle.monitorAgent != "qa-1" {
		t.Fatalf("monitor agent=%q", lifecycle.monitorAgent)
	}
	if lifecycle.monitorLines != 50 {
		t.Fatalf("monitor lines=%d", lifecycle.monitorLines)
	}
}

func TestTelegramMonitorMissingLifecycle(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}
	bot.httpClient = stubHTTPClient()
	bot.SetHandler(&fakeHandler{})

	msg := &TelegramMessage{
		Text: "/monitor qa-1",
		From: &TelegramUser{ID: 111},
		Chat: &TelegramChat{ID: 1},
	}

	handled, err := bot.handleCommand(msg)
	if !handled {
		t.Fatalf("expected handled")
	}
	if err == nil || err.Error() != "agent-manager is not available" {
		t.Fatalf("expected agent-manager not available error, got %v", err)
	}
}

func TestTelegramLifecycleAdminOnly(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}
	bot.httpClient = stubHTTPClient()
	bot.SetHandler(&fakeLifecycle{})

	cases := []string{"/startagent qa-1", "/stopagent qa-1", "/doctor"}
	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			msg := &TelegramMessage{
				Text: text,
				From: &TelegramUser{ID: 999},
				Chat: &TelegramChat{ID: 1},
			}
			handled, err := bot.handleCommand(msg)
			if !handled {
				t.Fatalf("expected handled")
			}
			if err == nil || err.Error() != "unauthorized: admin only" {
				t.Fatalf("expected admin-only error, got %v", err)
			}
		})
	}
}

func TestTelegramLifecycleErrorSanitized(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}
	bot.httpClient = stubHTTPClient()

	lifecycle := &fakeLifecycle{monitorErr: errors.New("boom details")}
	bot.SetHandler(lifecycle)

	msg := &TelegramMessage{
		Text: "/monitor qa-1",
		From: &TelegramUser{ID: 111},
		Chat: &TelegramChat{ID: 1},
	}

	handled, err := bot.handleCommand(msg)
	if !handled {
		t.Fatalf("expected handled")
	}
	if err == nil || err.Error() != "agent-manager error; please check server logs" {
		t.Fatalf("expected sanitized error, got %v", err)
	}
}

func TestTelegramLifecycleErrorSanitizedCommands(t *testing.T) {
	cases := []struct {
		name       string
		command    string
		setupError func(*fakeLifecycle)
	}{
		{
			name:    "startagent",
			command: "/startagent qa-1",
			setupError: func(l *fakeLifecycle) {
				l.startErr = errors.New("start failed")
			},
		},
		{
			name:    "stopagent",
			command: "/stopagent qa-1",
			setupError: func(l *fakeLifecycle) {
				l.stopErr = errors.New("stop failed")
			},
		},
		{
			name:    "doctor",
			command: "/doctor",
			setupError: func(l *fakeLifecycle) {
				l.doctorErr = errors.New("doctor failed")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bot, err := NewTelegramBot("token", nil, 123, "qa-1", []string{"qa-1"})
			if err != nil {
				t.Fatalf("NewTelegramBot: %v", err)
			}
			bot.httpClient = stubHTTPClient()

			lifecycle := &fakeLifecycle{}
			tc.setupError(lifecycle)
			bot.SetHandler(lifecycle)

			msg := &TelegramMessage{
				Text: tc.command,
				From: &TelegramUser{ID: 123},
				Chat: &TelegramChat{ID: 1},
			}

			handled, err := bot.handleCommand(msg)
			if !handled {
				t.Fatalf("expected handled")
			}
			if err == nil || err.Error() != "agent-manager error; please check server logs" {
				t.Fatalf("expected sanitized error, got %v", err)
			}
		})
	}
}
