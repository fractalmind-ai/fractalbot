package channels

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ErrorClass categorizes send errors for retry logic.
type ErrorClass int

const (
	// ErrPermanent errors are not retried (e.g., channel not running, invalid message).
	ErrPermanent ErrorClass = iota
	// ErrRateLimit errors are retried after a fixed 1s delay.
	ErrRateLimit
	// ErrTransient errors are retried with exponential backoff (500ms base, 8s max, 3 retries).
	ErrTransient
)

// classifyError determines whether a send error is permanent, rate-limited, or transient.
func classifyError(err error) ErrorClass {
	if err == nil {
		return ErrPermanent
	}
	msg := err.Error()
	// Rate limit patterns from platform APIs
	for _, pattern := range []string{"rate limit", "too many requests", "429", "slowmode"} {
		if containsLower(msg, pattern) {
			return ErrRateLimit
		}
	}
	// Token expiration is transient — recoverable by refreshing credentials.
	// Must be checked before the "invalid" permanent pattern below,
	// because "Invalid access token" contains "invalid".
	for _, pattern := range []string{"access token", "token expired", "99991663", "99991664", "99991671"} {
		if containsLower(msg, pattern) {
			return ErrTransient
		}
	}
	// Permanent failures — no point retrying
	for _, pattern := range []string{"not running", "not configured", "invalid", "unauthorized", "forbidden", "not found"} {
		if containsLower(msg, pattern) {
			return ErrPermanent
		}
	}
	// Default: treat as transient
	return ErrTransient
}

func containsLower(s, substr string) bool {
	ls := toLower(s)
	return len(ls) >= len(substr) && indexLower(ls, substr) >= 0
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func indexLower(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Platform rate limits (messages per second).
var platformRateLimits = map[string]rate.Limit{
	"telegram": 20,
	"discord":  1,
	"slack":    1,
	"feishu":   10,
	"imessage": 10,
}

const (
	workerQueueSize  = 16
	retryBaseDelay   = 500 * time.Millisecond
	retryMaxDelay    = 8 * time.Second
	retryMaxAttempts = 3
	rateLimitDelay   = 1 * time.Second
)

// channelWorker wraps a Channel with a buffered send queue, rate limiter,
// and classified retry logic. Each channel gets its own worker goroutine.
type channelWorker struct {
	channel Channel
	queue   chan OutboundMessage
	limiter *rate.Limiter
	done    chan struct{}
	wg      sync.WaitGroup

	// Placeholder state tracking (guarded by mu)
	mu           sync.Mutex
	placeholders map[string]*placeholderState // chatID → state
}

type placeholderState struct {
	messageID string
	stopTyp   func()
	undoReact func()
	createdAt time.Time
}

const (
	typingTTL      = 5 * time.Minute
	placeholderTTL = 10 * time.Minute
	janitorTick    = 10 * time.Second
)

// newChannelWorker creates a worker for the given channel.
// The rate limit is selected by channel name; unknown channels get 1 msg/s.
func newChannelWorker(ch Channel) *channelWorker {
	rl, ok := platformRateLimits[ch.Name()]
	if !ok {
		rl = 1
	}
	return &channelWorker{
		channel:      ch,
		queue:        make(chan OutboundMessage, workerQueueSize),
		limiter:      rate.NewLimiter(rl, int(math.Ceil(float64(rl)))),
		done:         make(chan struct{}),
		placeholders: make(map[string]*placeholderState),
	}
}

// start begins the worker goroutine and TTL janitor.
func (w *channelWorker) start(ctx context.Context) {
	w.wg.Add(2)
	go w.processLoop(ctx)
	go w.janitorLoop(ctx)
}

// stop signals the worker to drain remaining messages and exit.
func (w *channelWorker) stop() {
	close(w.done)
	w.wg.Wait()
}

// enqueue adds a message to the worker queue. Returns error if queue is full.
func (w *channelWorker) enqueue(msg OutboundMessage) error {
	select {
	case w.queue <- msg:
		return nil
	default:
		return fmt.Errorf("channel %s: send queue full (capacity %d)", w.channel.Name(), workerQueueSize)
	}
}

// processLoop is the main worker goroutine. It consumes from the queue,
// applies rate limiting, and retries on classified errors.
func (w *channelWorker) processLoop(ctx context.Context) {
	defer w.wg.Done()
	for {
		select {
		case <-w.done:
			// Drain remaining messages on shutdown
			w.drain(ctx)
			return
		case <-ctx.Done():
			return
		case msg := <-w.queue:
			w.sendWithRetry(ctx, msg)
		}
	}
}

// drain sends remaining queued messages without waiting.
func (w *channelWorker) drain(ctx context.Context) {
	for {
		select {
		case msg := <-w.queue:
			if _, err := w.channel.Send(ctx, msg); err != nil {
				log.Printf("[worker/%s] drain send failed: %v", w.channel.Name(), err)
			}
		default:
			return
		}
	}
}

// sendWithRetry attempts to send a message with rate limiting and classified retry.
func (w *channelWorker) sendWithRetry(ctx context.Context, msg OutboundMessage) {
	// Run placeholder pipeline before sending
	w.beforeSend(ctx, msg)

	// Rate limit
	if err := w.limiter.Wait(ctx); err != nil {
		log.Printf("[worker/%s] rate limiter cancelled: %v", w.channel.Name(), err)
		return
	}

	for attempt := 0; attempt <= retryMaxAttempts; attempt++ {
		_, err := w.channel.Send(ctx, msg)
		if err == nil {
			w.afterSend(ctx, msg)
			return
		}

		ec := classifyError(err)
		switch ec {
		case ErrPermanent:
			log.Printf("[worker/%s] permanent send error (no retry): %v", w.channel.Name(), err)
			return
		case ErrRateLimit:
			log.Printf("[worker/%s] rate limited, retry in %v: %v", w.channel.Name(), rateLimitDelay, err)
			select {
			case <-time.After(rateLimitDelay):
			case <-ctx.Done():
				return
			case <-w.done:
				return
			}
		case ErrTransient:
			if attempt >= retryMaxAttempts {
				log.Printf("[worker/%s] transient error, retries exhausted: %v", w.channel.Name(), err)
				return
			}
			delay := retryDelay(attempt)
			log.Printf("[worker/%s] transient error, retry %d/%d in %v: %v",
				w.channel.Name(), attempt+1, retryMaxAttempts, delay, err)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			case <-w.done:
				return
			}
		}
	}
}

// sendSync performs a synchronous send with rate limiting and returns the result.
// Used by Manager.Send for bus-originated sends that need result feedback.
func (w *channelWorker) sendSync(ctx context.Context, msg OutboundMessage) (*SendResult, error) {
	w.beforeSend(ctx, msg)

	if err := w.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	for attempt := 0; attempt <= retryMaxAttempts; attempt++ {
		result, err := w.channel.Send(ctx, msg)
		if err == nil {
			w.afterSend(ctx, msg)
			return result, nil
		}

		ec := classifyError(err)
		switch ec {
		case ErrPermanent:
			return nil, err
		case ErrRateLimit:
			select {
			case <-time.After(rateLimitDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case ErrTransient:
			if attempt >= retryMaxAttempts {
				return nil, err
			}
			delay := retryDelay(attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("channel %s: retries exhausted", w.channel.Name())
}

// retryDelay calculates exponential backoff: base * 2^attempt, capped at max.
func retryDelay(attempt int) time.Duration {
	delay := retryBaseDelay * time.Duration(1<<uint(attempt))
	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}
	return delay
}

// beforeSend runs the placeholder pipeline (typing, reaction, placeholder)
// if the channel supports it.
func (w *channelWorker) beforeSend(ctx context.Context, msg OutboundMessage) {
	chatID := msg.To

	// 1. Start typing indicator if supported
	if tc, ok := w.channel.(TypingCapable); ok {
		stop, err := tc.StartTyping(ctx, chatID)
		if err == nil && stop != nil {
			w.mu.Lock()
			ps := w.getOrCreatePlaceholder(chatID)
			ps.stopTyp = stop
			w.mu.Unlock()
		}
	}

	// 2. Send placeholder message if supported
	if pc, ok := w.channel.(PlaceholderCapable); ok {
		msgID, err := pc.SendPlaceholder(ctx, chatID, "⏳")
		if err == nil && msgID != "" {
			w.mu.Lock()
			ps := w.getOrCreatePlaceholder(chatID)
			ps.messageID = msgID
			// 3. Add reaction to placeholder if supported
			if rc, ok := w.channel.(ReactionCapable); ok {
				undo, err := rc.AddReaction(ctx, chatID, msgID, "⏳")
				if err == nil && undo != nil {
					ps.undoReact = undo
				}
			}
			w.mu.Unlock()
		}
	}
}

// afterSend cleans up placeholder state after a successful send.
func (w *channelWorker) afterSend(ctx context.Context, msg OutboundMessage) {
	chatID := msg.To

	w.mu.Lock()
	ps, ok := w.placeholders[chatID]
	if !ok {
		w.mu.Unlock()
		return
	}
	delete(w.placeholders, chatID)
	w.mu.Unlock()

	// 1. Stop typing indicator
	if ps.stopTyp != nil {
		ps.stopTyp()
	}

	// 2. Undo reaction
	if ps.undoReact != nil {
		ps.undoReact()
	}

	// 3. Edit placeholder → real response (if channel supports editing)
	if ps.messageID != "" {
		if ed, ok := w.channel.(MessageEditor); ok {
			if err := ed.EditMessage(ctx, chatID, ps.messageID, msg.Text); err != nil {
				log.Printf("[worker/%s] failed to edit placeholder: %v", w.channel.Name(), err)
			}
		}
	}
}

func (w *channelWorker) getOrCreatePlaceholder(chatID string) *placeholderState {
	ps, ok := w.placeholders[chatID]
	if !ok {
		ps = &placeholderState{createdAt: time.Now()}
		w.placeholders[chatID] = ps
	}
	return ps
}

// janitorLoop periodically cleans up expired placeholder state.
func (w *channelWorker) janitorLoop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(janitorTick)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanExpired()
		}
	}
}

func (w *channelWorker) cleanExpired() {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()

	for chatID, ps := range w.placeholders {
		age := now.Sub(ps.createdAt)
		expired := false

		// Typing indicators expire after 5 min
		if ps.stopTyp != nil && age > typingTTL {
			ps.stopTyp()
			ps.stopTyp = nil
			expired = true
		}

		// Placeholders expire after 10 min
		if ps.messageID != "" && age > placeholderTTL {
			if ps.undoReact != nil {
				ps.undoReact()
			}
			expired = true
		}

		if expired && ps.stopTyp == nil && ps.messageID == "" {
			delete(w.placeholders, chatID)
		} else if expired {
			// Reset the expired parts
			if age > placeholderTTL {
				delete(w.placeholders, chatID)
			}
		}
	}
}
