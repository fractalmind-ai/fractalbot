package bus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/fractalmind-ai/fractalbot/internal/channels"
	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

// ChannelSender routes outbound messages to the correct channel.
// Satisfied by channels.Manager.
type ChannelSender interface {
	Send(ctx context.Context, channelName string, msg channels.OutboundMessage) error
}

// InboundMessage wraps a protocol message with routing metadata.
type InboundMessage struct {
	Ctx         context.Context
	ChannelName string
	Message     *protocol.Message
	replyCh     chan inboundReply
}

type inboundReply struct {
	Text string
	Err  error
}

// OutboundEnvelope wraps an outbound message with its target channel and
// a completion channel for synchronous error reporting.
type OutboundEnvelope struct {
	Ctx         context.Context
	ChannelName string
	Message     channels.OutboundMessage
	errCh       chan error
}

// Stats holds message processing counters.
type Stats struct {
	InboundProcessed  int64
	OutboundProcessed int64
}

// MessageBus provides async in-process message routing between channels and
// the agent system. It decouples channel adapters from the agent router
// using buffered channels for both inbound and outbound message flows.
type MessageBus struct {
	inbound  chan *InboundMessage
	outbound chan *OutboundEnvelope
	closed   atomic.Bool
	wg       sync.WaitGroup

	inboundProcessed  atomic.Int64
	outboundProcessed atomic.Int64

	handler channels.IncomingMessageHandler
	sender  ChannelSender
}

// New creates a MessageBus with the given handler for inbound processing
// and sender for outbound routing. Buffer sizes control backpressure.
func New(handler channels.IncomingMessageHandler, sender ChannelSender, inboundBuf, outboundBuf int) *MessageBus {
	if inboundBuf < 0 {
		inboundBuf = 0
	}
	if outboundBuf < 0 {
		outboundBuf = 0
	}
	return &MessageBus{
		inbound:  make(chan *InboundMessage, inboundBuf),
		outbound: make(chan *OutboundEnvelope, outboundBuf),
		handler:  handler,
		sender:   sender,
	}
}

// Start launches the inbound and outbound consumer goroutines.
func (b *MessageBus) Start() {
	b.wg.Add(2)
	go b.consumeInbound()
	go b.consumeOutbound()
}

// HandleIncoming implements channels.IncomingMessageHandler.
// It publishes the message to the inbound pipe and blocks until the consumer
// processes it, preserving the synchronous request-reply contract.
func (b *MessageBus) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	if b.closed.Load() {
		return "", errors.New("bus is closed")
	}

	replyCh := make(chan inboundReply, 1)
	inMsg := &InboundMessage{
		Ctx:         ctx,
		ChannelName: extractChannelName(msg),
		Message:     msg,
		replyCh:     replyCh,
	}

	select {
	case b.inbound <- inMsg:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	select {
	case reply := <-replyCh:
		return reply.Text, reply.Err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// MonitorAgent forwards lifecycle monitor requests when the underlying handler
// supports channel agent lifecycle commands.
func (b *MessageBus) MonitorAgent(ctx context.Context, agentName string, lines int) (string, error) {
	lifecycle, ok := b.handler.(channels.AgentLifecycle)
	if !ok || lifecycle == nil {
		return "", errors.New("agent-manager is not available")
	}
	return lifecycle.MonitorAgent(ctx, agentName, lines)
}

// StartAgent forwards lifecycle start requests when available.
func (b *MessageBus) StartAgent(ctx context.Context, agentName string) (string, error) {
	lifecycle, ok := b.handler.(channels.AgentLifecycle)
	if !ok || lifecycle == nil {
		return "", errors.New("agent-manager is not available")
	}
	return lifecycle.StartAgent(ctx, agentName)
}

// StopAgent forwards lifecycle stop requests when available.
func (b *MessageBus) StopAgent(ctx context.Context, agentName string) (string, error) {
	lifecycle, ok := b.handler.(channels.AgentLifecycle)
	if !ok || lifecycle == nil {
		return "", errors.New("agent-manager is not available")
	}
	return lifecycle.StopAgent(ctx, agentName)
}

// Doctor forwards lifecycle doctor requests when available.
func (b *MessageBus) Doctor(ctx context.Context) (string, error) {
	lifecycle, ok := b.handler.(channels.AgentLifecycle)
	if !ok || lifecycle == nil {
		return "", errors.New("agent-manager is not available")
	}
	return lifecycle.Doctor(ctx)
}

// PublishOutbound sends an outbound message through the bus.
// It blocks until the consumer processes the send and returns the result.
func (b *MessageBus) PublishOutbound(ctx context.Context, channelName string, msg channels.OutboundMessage) error {
	if b.closed.Load() {
		return errors.New("bus is closed")
	}

	errCh := make(chan error, 1)
	env := &OutboundEnvelope{
		Ctx:         ctx,
		ChannelName: channelName,
		Message:     msg,
		errCh:       errCh,
	}

	select {
	case b.outbound <- env:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close signals both consumer goroutines to stop. It is safe to call
// multiple times.
func (b *MessageBus) Close() {
	if b.closed.CompareAndSwap(false, true) {
		close(b.inbound)
		close(b.outbound)
	}
}

// Wait blocks until both consumer goroutines have exited.
func (b *MessageBus) Wait() {
	b.wg.Wait()
}

// Stats returns message processing counters.
func (b *MessageBus) Stats() Stats {
	return Stats{
		InboundProcessed:  b.inboundProcessed.Load(),
		OutboundProcessed: b.outboundProcessed.Load(),
	}
}

func (b *MessageBus) consumeInbound() {
	defer b.wg.Done()
	for msg := range b.inbound {
		text, err := b.handler.HandleIncoming(msg.Ctx, msg.Message)
		msg.replyCh <- inboundReply{Text: text, Err: err}
		b.inboundProcessed.Add(1)
	}
}

func (b *MessageBus) consumeOutbound() {
	defer b.wg.Done()
	for env := range b.outbound {
		err := b.sender.Send(env.Ctx, env.ChannelName, env.Message)
		env.errCh <- err
		b.outboundProcessed.Add(1)
	}
}

// extractChannelName pulls the "channel" field from a protocol.Message's Data map.
func extractChannelName(msg *protocol.Message) string {
	if msg == nil || msg.Data == nil {
		return ""
	}
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := data["channel"].(string)
	return name
}

// Ensure MessageBus satisfies IncomingMessageHandler at compile time.
var _ channels.IncomingMessageHandler = (*MessageBus)(nil)
var _ channels.AgentLifecycle = (*MessageBus)(nil)

// String returns a human-readable description for debugging.
func (b *MessageBus) String() string {
	state := "running"
	if b.closed.Load() {
		state = "closed"
	}
	return fmt.Sprintf("MessageBus(%s, inbound=%d/%d, outbound=%d/%d)",
		state,
		len(b.inbound), cap(b.inbound),
		len(b.outbound), cap(b.outbound))
}
