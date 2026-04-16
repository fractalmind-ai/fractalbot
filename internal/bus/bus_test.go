package bus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/internal/channels"
	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

// --- test helpers ---

type fakeHandler struct {
	mu      sync.Mutex
	calls   int
	lastMsg *protocol.Message
	replyFn func(ctx context.Context, msg *protocol.Message) (string, error)
}

func (h *fakeHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	h.mu.Lock()
	h.calls++
	h.lastMsg = msg
	fn := h.replyFn
	h.mu.Unlock()
	if fn != nil {
		return fn(ctx, msg)
	}
	return "ack", nil
}

func (h *fakeHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

type fakeLifecycleHandler struct {
	fakeHandler
	monitorReply string
	monitorErr   error
	startReply   string
	startErr     error
	stopReply    string
	stopErr      error
	doctorReply  string
	doctorErr    error
}

func (h *fakeLifecycleHandler) MonitorAgent(ctx context.Context, agentName string, lines int) (string, error) {
	return h.monitorReply, h.monitorErr
}

func (h *fakeLifecycleHandler) StartAgent(ctx context.Context, agentName string) (string, error) {
	return h.startReply, h.startErr
}

func (h *fakeLifecycleHandler) StopAgent(ctx context.Context, agentName string) (string, error) {
	return h.stopReply, h.stopErr
}

func (h *fakeLifecycleHandler) Doctor(ctx context.Context) (string, error) {
	return h.doctorReply, h.doctorErr
}

type fakeSender struct {
	mu      sync.Mutex
	calls   int
	lastCh  string
	lastMsg channels.OutboundMessage
	sendErr error
}

func (s *fakeSender) Send(ctx context.Context, channelName string, msg channels.OutboundMessage) (*channels.SendResult, error) {
	s.mu.Lock()
	s.calls++
	s.lastCh = channelName
	s.lastMsg = msg
	err := s.sendErr
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &channels.SendResult{ChannelID: msg.To}, nil
}

func (s *fakeSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func makeProtocolMsg(channel, text string) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel": channel,
			"text":    text,
		},
	}
}

// --- tests ---

func TestInboundPublishAndConsume(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 8, 8)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	msg := makeProtocolMsg("telegram", "hello")
	reply, err := b.HandleIncoming(context.Background(), msg)
	if err != nil {
		t.Fatalf("HandleIncoming: %v", err)
	}
	if reply != "ack" {
		t.Fatalf("expected reply 'ack', got %q", reply)
	}
	if handler.callCount() != 1 {
		t.Fatalf("expected 1 handler call, got %d", handler.callCount())
	}
}

func TestInboundExtractsChannelName(t *testing.T) {
	var gotName string
	handler := &fakeHandler{
		replyFn: func(ctx context.Context, msg *protocol.Message) (string, error) {
			return "ok", nil
		},
	}
	sender := &fakeSender{}

	// Override: inspect the InboundMessage directly via extractChannelName
	msg := makeProtocolMsg("slack", "test")
	name := extractChannelName(msg)
	if name != "slack" {
		t.Fatalf("expected channel name 'slack', got %q", name)
	}
	_ = gotName

	b := New(handler, sender, 4, 4)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	reply, err := b.HandleIncoming(context.Background(), msg)
	if err != nil {
		t.Fatalf("HandleIncoming: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("expected reply 'ok', got %q", reply)
	}
}

func TestInboundHandlerError(t *testing.T) {
	handler := &fakeHandler{
		replyFn: func(ctx context.Context, msg *protocol.Message) (string, error) {
			return "", errors.New("handler boom")
		},
	}
	sender := &fakeSender{}
	b := New(handler, sender, 4, 4)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	_, err := b.HandleIncoming(context.Background(), makeProtocolMsg("telegram", "x"))
	if err == nil || err.Error() != "handler boom" {
		t.Fatalf("expected 'handler boom' error, got %v", err)
	}
}

func TestOutboundPublishAndConsume(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 4, 4)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	result, err := b.PublishOutbound(context.Background(), "telegram", channels.OutboundMessage{
		To:   "12345",
		Text: "hello from bus",
	})
	if err != nil {
		t.Fatalf("PublishOutbound: %v", err)
	}
	if result == nil || result.ChannelID != "12345" {
		t.Fatalf("expected SendResult with ChannelID=12345, got %v", result)
	}
	if sender.callCount() != 1 {
		t.Fatalf("expected 1 sender call, got %d", sender.callCount())
	}
	sender.mu.Lock()
	if sender.lastCh != "telegram" {
		t.Fatalf("expected channel 'telegram', got %q", sender.lastCh)
	}
	if sender.lastMsg.To != "12345" {
		t.Fatalf("expected to '12345', got %q", sender.lastMsg.To)
	}
	if sender.lastMsg.Text != "hello from bus" {
		t.Fatalf("expected text 'hello from bus', got %q", sender.lastMsg.Text)
	}
	sender.mu.Unlock()
}

func TestOutboundSendError(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{sendErr: errors.New("send failed")}
	b := New(handler, sender, 4, 4)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	_, err := b.PublishOutbound(context.Background(), "slack", channels.OutboundMessage{
		To:   "C123",
		Text: "test",
	})
	if err == nil || err.Error() != "send failed" {
		t.Fatalf("expected 'send failed' error, got %v", err)
	}
}

func TestOutboundWithThreadTS(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 4, 4)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	_, err := b.PublishOutbound(context.Background(), "slack", channels.OutboundMessage{
		To:       "C123",
		Text:     "reply",
		ThreadTS: "1234567890.123456",
	})
	if err != nil {
		t.Fatalf("PublishOutbound: %v", err)
	}
	sender.mu.Lock()
	if sender.lastMsg.ThreadTS != "1234567890.123456" {
		t.Fatalf("expected thread ts, got %q", sender.lastMsg.ThreadTS)
	}
	sender.mu.Unlock()
}

func TestPublishInboundContextCanceled(t *testing.T) {
	handler := &fakeHandler{
		replyFn: func(ctx context.Context, msg *protocol.Message) (string, error) {
			time.Sleep(time.Second) // slow handler
			return "late", nil
		},
	}
	sender := &fakeSender{}
	// buffer=0 so publish blocks until consumer picks up
	b := New(handler, sender, 0, 0)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := b.HandleIncoming(ctx, makeProtocolMsg("telegram", "x"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestPublishOutboundContextCanceled(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	// buffer=0 so publish blocks until consumer picks up
	b := New(handler, sender, 0, 0)
	// Don't start consumers — publish will block on channel send
	// (We need to start, but make sender slow instead)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	// Make sender block
	sender.sendErr = nil
	origSend := sender.Send
	_ = origSend
	slowSender := &fakeSender{}
	b2 := New(handler, slowSender, 0, 0)
	// Don't start b2 — no consumer, so publish blocks on channel write
	defer b2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := b2.PublishOutbound(ctx, "telegram", channels.OutboundMessage{To: "1", Text: "x"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestClosePreventsFurtherPublish(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 4, 4)
	b.Start()
	b.Close()
	b.Wait()

	_, err := b.HandleIncoming(context.Background(), makeProtocolMsg("telegram", "x"))
	if err == nil || err.Error() != "bus is closed" {
		t.Fatalf("expected 'bus is closed', got %v", err)
	}

	_, err = b.PublishOutbound(context.Background(), "telegram", channels.OutboundMessage{To: "1", Text: "x"})
	if err == nil || err.Error() != "bus is closed" {
		t.Fatalf("expected 'bus is closed', got %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 4, 4)
	b.Start()

	// Should not panic
	b.Close()
	b.Close()
	b.Close()
	b.Wait()
}

func TestCloseDrainsInFlight(t *testing.T) {
	var processed atomic.Int32
	handler := &fakeHandler{
		replyFn: func(ctx context.Context, msg *protocol.Message) (string, error) {
			processed.Add(1)
			return "ok", nil
		},
	}
	sender := &fakeSender{}
	b := New(handler, sender, 8, 8)
	b.Start()

	// Publish several messages
	const n = 5
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			_, _ = b.HandleIncoming(context.Background(), makeProtocolMsg("telegram", "msg"))
		}
	}()
	<-done

	b.Close()
	b.Wait()

	if got := processed.Load(); got != n {
		t.Fatalf("expected %d processed, got %d", n, got)
	}
}

func TestConcurrentPublish(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 64, 64)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	const goroutines = 10
	const perGoroutine = 5
	var wg sync.WaitGroup

	// Concurrent inbound
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_, err := b.HandleIncoming(context.Background(), makeProtocolMsg("telegram", "x"))
				if err != nil {
					t.Errorf("inbound error: %v", err)
				}
			}
		}()
	}

	// Concurrent outbound
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_, err := b.PublishOutbound(context.Background(), "telegram", channels.OutboundMessage{To: "1", Text: "x"})
				if err != nil {
					t.Errorf("outbound error: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	expected := goroutines * perGoroutine
	if got := handler.callCount(); got != expected {
		t.Fatalf("expected %d handler calls, got %d", expected, got)
	}
	if got := sender.callCount(); got != expected {
		t.Fatalf("expected %d sender calls, got %d", expected, got)
	}
}

func TestExtractChannelName(t *testing.T) {
	tests := []struct {
		name string
		msg  *protocol.Message
		want string
	}{
		{"nil message", nil, ""},
		{"nil data", &protocol.Message{}, ""},
		{"non-map data", &protocol.Message{Data: "string"}, ""},
		{"missing channel key", &protocol.Message{Data: map[string]interface{}{"text": "hello"}}, ""},
		{"telegram", makeProtocolMsg("telegram", "hi"), "telegram"},
		{"slack", makeProtocolMsg("slack", "hi"), "slack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractChannelName(tt.msg)
			if got != tt.want {
				t.Fatalf("extractChannelName()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestNegativeBufferSizesClamped(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, -5, -10)
	if cap(b.inbound) != 0 {
		t.Fatalf("expected inbound cap=0, got %d", cap(b.inbound))
	}
	if cap(b.outbound) != 0 {
		t.Fatalf("expected outbound cap=0, got %d", cap(b.outbound))
	}
}

func TestString(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 8, 16)
	s := b.String()
	if s == "" {
		t.Fatal("String() returned empty")
	}
	// Verify it contains key info
	if !containsAll(s, "MessageBus", "running", "inbound", "outbound") {
		t.Fatalf("unexpected String() output: %s", s)
	}

	b.Close()
	s = b.String()
	if !containsAll(s, "closed") {
		t.Fatalf("expected 'closed' in String(), got: %s", s)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		found := false
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestStatsCountsMessages(t *testing.T) {
	handler := &fakeHandler{}
	sender := &fakeSender{}
	b := New(handler, sender, 8, 8)
	b.Start()
	defer func() { b.Close(); b.Wait() }()

	stats := b.Stats()
	if stats.InboundProcessed != 0 || stats.OutboundProcessed != 0 {
		t.Fatalf("expected zero counts initially, got in=%d out=%d", stats.InboundProcessed, stats.OutboundProcessed)
	}

	// Send 3 inbound messages
	for i := 0; i < 3; i++ {
		if _, err := b.HandleIncoming(context.Background(), makeProtocolMsg("telegram", "msg")); err != nil {
			t.Fatalf("HandleIncoming: %v", err)
		}
	}

	// Send 2 outbound messages
	for i := 0; i < 2; i++ {
		if _, err := b.PublishOutbound(context.Background(), "telegram", channels.OutboundMessage{To: "1", Text: "x"}); err != nil {
			t.Fatalf("PublishOutbound: %v", err)
		}
	}

	stats = b.Stats()
	if stats.InboundProcessed != 3 {
		t.Fatalf("expected 3 inbound processed, got %d", stats.InboundProcessed)
	}
	if stats.OutboundProcessed != 2 {
		t.Fatalf("expected 2 outbound processed, got %d", stats.OutboundProcessed)
	}
}

func TestLifecycleDelegatesToUnderlyingHandler(t *testing.T) {
	handler := &fakeLifecycleHandler{
		monitorReply: "monitor ok",
		startReply:   "start ok",
		stopReply:    "stop ok",
		doctorReply:  "doctor ok",
	}
	b := New(handler, &fakeSender{}, 1, 1)

	if got, err := b.MonitorAgent(context.Background(), "main", 10); err != nil || got != "monitor ok" {
		t.Fatalf("MonitorAgent() = %q, %v", got, err)
	}
	if got, err := b.StartAgent(context.Background(), "main"); err != nil || got != "start ok" {
		t.Fatalf("StartAgent() = %q, %v", got, err)
	}
	if got, err := b.StopAgent(context.Background(), "main"); err != nil || got != "stop ok" {
		t.Fatalf("StopAgent() = %q, %v", got, err)
	}
	if got, err := b.Doctor(context.Background()); err != nil || got != "doctor ok" {
		t.Fatalf("Doctor() = %q, %v", got, err)
	}
}

func TestLifecycleReturnsNotAvailableWithoutUnderlyingLifecycle(t *testing.T) {
	b := New(&fakeHandler{}, &fakeSender{}, 1, 1)

	checks := []struct {
		name string
		call func() error
	}{
		{
			name: "monitor",
			call: func() error {
				_, err := b.MonitorAgent(context.Background(), "main", 10)
				return err
			},
		},
		{
			name: "start",
			call: func() error {
				_, err := b.StartAgent(context.Background(), "main")
				return err
			},
		},
		{
			name: "stop",
			call: func() error {
				_, err := b.StopAgent(context.Background(), "main")
				return err
			},
		},
		{
			name: "doctor",
			call: func() error {
				_, err := b.Doctor(context.Background())
				return err
			},
		},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil || err.Error() != "agent-manager is not available" {
				t.Fatalf("expected agent-manager unavailable, got %v", err)
			}
		})
	}
}
