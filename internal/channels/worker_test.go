package channels

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Test helpers ---

type stubChannel struct {
	name    string
	running bool
	sendFn  func(ctx context.Context, msg OutboundMessage) error
}

func (s *stubChannel) Name() string                    { return s.name }
func (s *stubChannel) Start(ctx context.Context) error { s.running = true; return nil }
func (s *stubChannel) Stop(ctx context.Context) error  { s.running = false; return nil }
func (s *stubChannel) IsRunning() bool                 { return s.running }
func (s *stubChannel) IsAllowed(senderID string) bool  { return true }
func (s *stubChannel) Send(ctx context.Context, msg OutboundMessage) error {
	if s.sendFn != nil {
		return s.sendFn(ctx, msg)
	}
	return nil
}

// --- classifyError tests ---

func TestClassifyError_Nil(t *testing.T) {
	if got := classifyError(nil); got != ErrPermanent {
		t.Errorf("classifyError(nil) = %d, want %d (ErrPermanent)", got, ErrPermanent)
	}
}

func TestClassifyError_RateLimit(t *testing.T) {
	tests := []string{
		"rate limit exceeded",
		"429 Too Many Requests",
		"too many requests",
		"slowmode is active",
	}
	for _, msg := range tests {
		if got := classifyError(errors.New(msg)); got != ErrRateLimit {
			t.Errorf("classifyError(%q) = %d, want %d (ErrRateLimit)", msg, got, ErrRateLimit)
		}
	}
}

func TestClassifyError_Permanent(t *testing.T) {
	tests := []string{
		"channel not running",
		"sender not configured",
		"invalid telegram chat ID",
		"unauthorized",
		"forbidden",
		"not found",
	}
	for _, msg := range tests {
		if got := classifyError(errors.New(msg)); got != ErrPermanent {
			t.Errorf("classifyError(%q) = %d, want %d (ErrPermanent)", msg, got, ErrPermanent)
		}
	}
}

func TestClassifyError_Transient(t *testing.T) {
	tests := []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"server error",
	}
	for _, msg := range tests {
		if got := classifyError(errors.New(msg)); got != ErrTransient {
			t.Errorf("classifyError(%q) = %d, want %d (ErrTransient)", msg, got, ErrTransient)
		}
	}
}

// --- retryDelay tests ---

func TestRetryDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 500 * time.Millisecond}, // base
		{1, 1 * time.Second},        // 2x
		{2, 2 * time.Second},        // 4x
		{3, 4 * time.Second},        // 8x
		{4, 8 * time.Second},        // 16x → capped at 8s
		{10, 8 * time.Second},       // cap
	}
	for _, tt := range tests {
		got := retryDelay(tt.attempt)
		if got != tt.want {
			t.Errorf("retryDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

// --- Worker lifecycle tests ---

func TestWorker_EnqueueAndProcess(t *testing.T) {
	var received []OutboundMessage
	var mu sync.Mutex

	ch := &stubChannel{
		name: "telegram",
		sendFn: func(ctx context.Context, msg OutboundMessage) error {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
			return nil
		},
	}

	w := newChannelWorker(ch)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.start(ctx)

	// Enqueue 3 messages
	for i := 0; i < 3; i++ {
		if err := w.enqueue(OutboundMessage{To: "chat1", Text: "hello"}); err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)
	w.stop()

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Errorf("got %d messages, want 3", len(received))
	}
}

func TestWorker_QueueFull(t *testing.T) {
	ch := &stubChannel{name: "discord"}

	w := newChannelWorker(ch)
	// Don't start the worker — the queue channel will fill up without being consumed.

	// Fill the queue (capacity is workerQueueSize = 16)
	for i := 0; i < workerQueueSize; i++ {
		if err := w.enqueue(OutboundMessage{To: "chat", Text: "msg"}); err != nil {
			t.Fatalf("enqueue %d failed unexpectedly: %v", i, err)
		}
	}

	// Next enqueue should fail
	err := w.enqueue(OutboundMessage{To: "chat", Text: "overflow"})
	if err == nil {
		t.Error("expected error when queue is full, got nil")
	}
}

func TestWorker_PermanentErrorNoRetry(t *testing.T) {
	var sendCount int32

	ch := &stubChannel{
		name: "slack",
		sendFn: func(ctx context.Context, msg OutboundMessage) error {
			atomic.AddInt32(&sendCount, 1)
			return errors.New("channel not running")
		},
	}

	w := newChannelWorker(ch)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.start(ctx)

	w.enqueue(OutboundMessage{To: "ch1", Text: "test"})
	time.Sleep(200 * time.Millisecond)
	w.stop()

	if got := atomic.LoadInt32(&sendCount); got != 1 {
		t.Errorf("permanent error: send called %d times, want 1 (no retry)", got)
	}
}

func TestWorker_TransientErrorRetries(t *testing.T) {
	var sendCount int32

	ch := &stubChannel{
		name: "feishu",
		sendFn: func(ctx context.Context, msg OutboundMessage) error {
			n := atomic.AddInt32(&sendCount, 1)
			if n <= 2 {
				return errors.New("connection refused")
			}
			return nil // Succeed on 3rd attempt
		},
	}

	w := newChannelWorker(ch)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.start(ctx)

	w.enqueue(OutboundMessage{To: "ch1", Text: "test"})
	// Wait for retries (500ms + 1s + processing time)
	time.Sleep(3 * time.Second)
	w.stop()

	if got := atomic.LoadInt32(&sendCount); got != 3 {
		t.Errorf("transient error: send called %d times, want 3", got)
	}
}

func TestWorker_RateLimiterApplied(t *testing.T) {
	var times []time.Time
	var mu sync.Mutex

	ch := &stubChannel{
		name: "discord", // 1 msg/s rate limit
		sendFn: func(ctx context.Context, msg OutboundMessage) error {
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
			return nil
		},
	}

	w := newChannelWorker(ch)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.start(ctx)

	// Send 3 messages rapidly
	for i := 0; i < 3; i++ {
		w.enqueue(OutboundMessage{To: "ch1", Text: "test"})
	}

	// Wait for processing with rate limit (discord: 1/s)
	time.Sleep(4 * time.Second)
	w.stop()

	mu.Lock()
	defer mu.Unlock()

	if len(times) < 3 {
		t.Fatalf("only %d messages processed, want 3", len(times))
	}

	// Verify at least 1 second gap between 2nd and 3rd messages
	// (1st may be immediate due to burst allowance)
	gap := times[2].Sub(times[0])
	if gap < 1*time.Second {
		t.Errorf("messages processed too quickly (gap=%v), rate limiting not effective", gap)
	}
}

// --- Platform rate limit tests ---

func TestPlatformRateLimits(t *testing.T) {
	tests := []struct {
		name     string
		wantRate float64
	}{
		{"telegram", 20},
		{"discord", 1},
		{"slack", 1},
		{"feishu", 10},
		{"imessage", 10},
	}
	for _, tt := range tests {
		rl, ok := platformRateLimits[tt.name]
		if !ok {
			t.Errorf("no rate limit defined for %q", tt.name)
			continue
		}
		if float64(rl) != tt.wantRate {
			t.Errorf("rate limit for %q = %v, want %v", tt.name, rl, tt.wantRate)
		}
	}
}

func TestWorker_UnknownPlatformDefaultsTo1(t *testing.T) {
	ch := &stubChannel{name: "unknown_platform"}
	w := newChannelWorker(ch)
	// Check the limiter limit is 1 (default)
	if w.limiter.Limit() != 1 {
		t.Errorf("unknown platform rate limit = %v, want 1", w.limiter.Limit())
	}
}

// --- Manager.Send integration ---

func TestManager_Send_RoutesToWorker(t *testing.T) {
	var received atomic.Int32
	ch := &stubChannel{
		name:    "test",
		running: true,
		sendFn: func(ctx context.Context, msg OutboundMessage) error {
			received.Add(1)
			return nil
		},
	}

	mgr := &Manager{
		channels:     map[string]Channel{"test": ch},
		workers:      make(map[string]*channelWorker),
		startCancels: make(map[string]context.CancelFunc),
	}

	// Create and start worker
	w := newChannelWorker(ch)
	mgr.workers["test"] = w
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.start(ctx)

	err := mgr.Send(context.Background(), "test", OutboundMessage{To: "chat1", Text: "hi"})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	w.stop()

	if received.Load() != 1 {
		t.Errorf("worker received %d messages, want 1", received.Load())
	}
}

func TestManager_Send_FallbackDirect(t *testing.T) {
	var directSent bool
	ch := &stubChannel{
		name: "test",
		sendFn: func(ctx context.Context, msg OutboundMessage) error {
			directSent = true
			return nil
		},
	}

	mgr := &Manager{
		channels:     map[string]Channel{"test": ch},
		workers:      make(map[string]*channelWorker),
		startCancels: make(map[string]context.CancelFunc),
	}
	// No worker started — should fallback to direct send

	err := mgr.Send(context.Background(), "test", OutboundMessage{To: "chat1", Text: "hi"})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if !directSent {
		t.Error("expected direct send fallback when worker not started")
	}
}

func TestManager_Send_UnknownChannel(t *testing.T) {
	mgr := &Manager{
		channels:     make(map[string]Channel),
		workers:      make(map[string]*channelWorker),
		startCancels: make(map[string]context.CancelFunc),
	}

	err := mgr.Send(context.Background(), "nonexistent", OutboundMessage{})
	if err == nil {
		t.Error("expected error for unknown channel")
	}
}

// --- Drain on stop ---

func TestWorker_DrainOnStop(t *testing.T) {
	var count atomic.Int32
	ch := &stubChannel{
		name: "telegram",
		sendFn: func(ctx context.Context, msg OutboundMessage) error {
			count.Add(1)
			return nil
		},
	}

	w := newChannelWorker(ch)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Enqueue before starting so messages are in the buffer
	for i := 0; i < 5; i++ {
		w.enqueue(OutboundMessage{To: "chat", Text: "drain"})
	}

	w.start(ctx)
	time.Sleep(100 * time.Millisecond) // Let worker start
	w.stop()                           // Should drain remaining

	if got := count.Load(); got != 5 {
		t.Errorf("drain: sent %d messages, want 5", got)
	}
}
