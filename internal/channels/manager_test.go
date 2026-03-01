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

func (f *fakeChannel) Stop() error {
	f.stopped++
	f.running = false
	return f.stopErr
}

func (f *fakeChannel) SendMessage(ctx context.Context, target string, text string) error {
	_ = ctx
	f.lastChat = target
	f.lastText = text
	return nil
}

func (f *fakeChannel) IsRunning() bool { return f.running }

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

func (b *blockingStartChannel) Stop() error { return nil }

func (b *blockingStartChannel) SendMessage(ctx context.Context, target string, text string) error {
	_ = ctx
	_ = target
	_ = text
	return nil
}

func (b *blockingStartChannel) IsRunning() bool { return false }

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
