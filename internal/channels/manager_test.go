package channels

import (
	"context"
	"testing"
)

type fakeChannel struct {
	name     string
	started  int
	stopped  int
	running  bool
	startErr error
	stopErr  error
	lastChat int64
	lastText string
}

func (f *fakeChannel) Name() string { return f.name }

func (f *fakeChannel) Start(ctx context.Context) error {
	_ = ctx
	f.started++
	f.running = true
	return f.startErr
}

func (f *fakeChannel) Stop() error {
	f.stopped++
	f.running = false
	return f.stopErr
}

func (f *fakeChannel) SendMessage(ctx context.Context, chatID int64, text string) error {
	_ = ctx
	f.lastChat = chatID
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
