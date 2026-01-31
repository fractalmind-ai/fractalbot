package channels

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTelegramPollingBackoff(t *testing.T) {
	bot, err := NewTelegramBot("token", nil, 0, "", nil)
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	bot.ctx = ctx
	bot.cancel = cancel

	var calls int32
	bot.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&calls, 1)
			switch count {
			case 1, 2:
				return nil, errors.New("boom")
			case 3:
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":[]}`)),
					Header:     make(http.Header),
				}, nil
			case 4:
				cancel()
				return nil, errors.New("boom")
			default:
				return nil, errors.New("unexpected call")
			}
		}),
	}

	durations := make(chan time.Duration, 10)
	bot.sleeper = func(d time.Duration) {
		durations <- d
	}

	bot.startPollingLoop()

	got := make([]time.Duration, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case d := <-durations:
			got = append(got, d)
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for backoff sleep")
		}
	}

	want := []time.Duration{
		defaultTelegramPollingBackoffMin,
		defaultTelegramPollingBackoffMin * 2,
		defaultTelegramPollingBackoffMin,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backoff durations=%v want=%v", got, want)
	}
}
