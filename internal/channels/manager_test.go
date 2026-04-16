package channels

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeChannel struct {
	name     string
	started  int
	stopped  int
	running  bool
	startErr error
	stopErr  error
	lastChat string
	lastText string
}

func (f *fakeChannel) Name() string { return f.name }

func (f *fakeChannel) Start(ctx context.Context) error {
	_ = ctx
	f.started++
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}

func (f *fakeChannel) Stop(ctx context.Context) error {
	_ = ctx
	f.stopped++
	f.running = false
	return f.stopErr
}

func (f *fakeChannel) Send(ctx context.Context, msg OutboundMessage) (*SendResult, error) {
	_ = ctx
	f.lastChat = msg.To
	f.lastText = msg.Text
	return &SendResult{ChannelID: msg.To}, nil
}

func (f *fakeChannel) IsRunning() bool { return f.running }

func (f *fakeChannel) IsAllowed(senderID string) bool { return true }

func TestManagerStartStop(t *testing.T) {
	manager := NewManager(nil, nil)
	fake := &fakeChannel{name: "fake"}

	if err := manager.Register(fake); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, time.Second, func() bool {
		return fake.running && fake.started == 1
	})
	if !fake.running || fake.started != 1 {
		t.Fatalf("expected channel started, running=%v started=%d", fake.running, fake.started)
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if fake.running || fake.stopped != 1 {
		t.Fatalf("expected channel stopped, running=%v stopped=%d", fake.running, fake.stopped)
	}
}

func TestManagerRegisterDuplicate(t *testing.T) {
	manager := NewManager(nil, nil)
	fake := &fakeChannel{name: "fake"}

	if err := manager.Register(fake); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := manager.Register(fake); err == nil {
		t.Fatalf("expected duplicate register error")
	}
}

func TestManagerStartBestEffortWhenOneChannelFails(t *testing.T) {
	manager := NewManager(nil, nil)
	good := &fakeChannel{name: "good"}
	bad := &fakeChannel{name: "bad", startErr: errors.New("boom")}

	if err := manager.Register(good); err != nil {
		t.Fatalf("register good: %v", err)
	}
	if err := manager.Register(bad); err != nil {
		t.Fatalf("register bad: %v", err)
	}

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start should be best-effort, got error: %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		return good.started == 1 && bad.started == 1
	})

	if !good.running {
		t.Fatalf("expected good channel to start")
	}
	if bad.running {
		t.Fatalf("expected bad channel not running")
	}
}

func TestManagerStartDoesNotBlockOnSlowChannel(t *testing.T) {
	manager := NewManager(nil, nil)
	blockCtxObserved := make(chan struct{})
	quick := &fakeChannel{name: "quick"}
	blocking := &blockingStartChannel{
		name:           "blocking",
		started:        make(chan struct{}),
		blockCtxSignal: blockCtxObserved,
	}

	if err := manager.Register(quick); err != nil {
		t.Fatalf("register quick: %v", err)
	}
	if err := manager.Register(blocking); err != nil {
		t.Fatalf("register blocking: %v", err)
	}

	startedAt := time.Now()
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 200*time.Millisecond {
		t.Fatalf("manager start blocked for %s", elapsed)
	}

	waitForCondition(t, time.Second, func() bool { return quick.started == 1 })

	if err := manager.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	select {
	case <-blockCtxObserved:
	case <-time.After(time.Second):
		t.Fatalf("expected blocking start context cancellation on stop")
	}
}

type blockingStartChannel struct {
	name           string
	started        chan struct{}
	blockCtxSignal chan struct{}
}

func (b *blockingStartChannel) Name() string { return b.name }

func (b *blockingStartChannel) Start(ctx context.Context) error {
	close(b.started)
	<-ctx.Done()
	close(b.blockCtxSignal)
	return ctx.Err()
}

func (b *blockingStartChannel) Stop(ctx context.Context) error { _ = ctx; return nil }

func (b *blockingStartChannel) Send(ctx context.Context, msg OutboundMessage) (*SendResult, error) {
	_ = ctx
	return nil, nil
}

func (b *blockingStartChannel) IsRunning() bool { return false }

func (b *blockingStartChannel) IsAllowed(senderID string) bool { return true }

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout (%s)", timeout)
}

func TestManagerStartUsesIndependentContexts(t *testing.T) {
	// When the parent context is canceled, channels with independent contexts
	// should NOT have their contexts canceled automatically. They should only
	// stop when Stop() is called explicitly.
	manager := NewManager(nil, nil)
	ctxObserved := make(chan struct{})
	ch := &contextObservingChannel{
		name:      "observer",
		ctxDoneCh: ctxObserved,
		startedCh: make(chan struct{}),
	}

	if err := manager.Register(ch); err != nil {
		t.Fatalf("register: %v", err)
	}

	parentCtx, parentCancel := context.WithCancel(context.Background())
	if err := manager.Start(parentCtx); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait for the channel to start
	select {
	case <-ch.startedCh:
	case <-time.After(time.Second):
		t.Fatalf("channel did not start in time")
	}

	// Cancel the parent context — channel should NOT be affected
	parentCancel()

	// Give it a moment to propagate (if it were a child, it would be canceled)
	select {
	case <-ctxObserved:
		t.Fatalf("channel context was canceled when parent was canceled — contexts are not isolated")
	case <-time.After(100 * time.Millisecond):
		// Good — channel context was NOT canceled
	}

	// Now explicitly stop — this should cancel the channel
	if err := manager.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestManagerStartChannelPanicRecovery(t *testing.T) {
	manager := NewManager(nil, nil)
	good := &fakeChannel{name: "good"}
	panicking := &panickingChannel{name: "panicky", panicDone: make(chan struct{})}

	if err := manager.Register(good); err != nil {
		t.Fatalf("register good: %v", err)
	}
	if err := manager.Register(panicking); err != nil {
		t.Fatalf("register panicky: %v", err)
	}

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait for both channels to attempt start
	waitForCondition(t, time.Second, func() bool {
		return good.started == 1
	})

	select {
	case <-panicking.panicDone:
	case <-time.After(time.Second):
		t.Fatalf("panicking channel did not run")
	}

	// Good channel should still be running despite the other panicking
	if !good.running {
		t.Fatalf("expected good channel to be running after sibling panic")
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// contextObservingChannel blocks on Start until its context is canceled,
// and signals on ctxDoneCh when that happens.
type contextObservingChannel struct {
	name      string
	ctxDoneCh chan struct{}
	startedCh chan struct{}
	running   bool
}

func (c *contextObservingChannel) Name() string { return c.name }

func (c *contextObservingChannel) Start(ctx context.Context) error {
	c.running = true
	close(c.startedCh)
	<-ctx.Done()
	close(c.ctxDoneCh)
	c.running = false
	return ctx.Err()
}

func (c *contextObservingChannel) Stop(ctx context.Context) error {
	_ = ctx
	c.running = false
	return nil
}

func (c *contextObservingChannel) Send(ctx context.Context, msg OutboundMessage) (*SendResult, error) {
	return nil, nil
}

func (c *contextObservingChannel) IsRunning() bool { return c.running }

func (c *contextObservingChannel) IsAllowed(senderID string) bool { return true }

// panickingChannel panics during Start to test panic recovery.
type panickingChannel struct {
	name      string
	panicDone chan struct{}
}

func (p *panickingChannel) Name() string { return p.name }

func (p *panickingChannel) Start(ctx context.Context) error {
	defer close(p.panicDone)
	panic("test panic in channel start")
}

func (p *panickingChannel) Stop(ctx context.Context) error { _ = ctx; return nil }
func (p *panickingChannel) IsRunning() bool                { return false }
func (p *panickingChannel) IsAllowed(senderID string) bool { return true }
func (p *panickingChannel) Send(ctx context.Context, msg OutboundMessage) (*SendResult, error) {
	return nil, nil
}
