package channels

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractal-demail/client-go/envelope"
	"github.com/fractalmind-ai/fractal-demail/client-go/listener"
	"github.com/fractalmind-ai/fractal-demail/client-go/schema"
	gasstation "github.com/fractalmind-ai/fractal-demail/gas-station-adapter"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const (
	defaultDemailPollInterval = 2 * time.Second
	demailClockObjectID       = "0x6"
	demailGasBudget           = "10000000"
)

// DemailOptions configures the demail channel.
type DemailOptions struct {
	// RPCURL is the Sui JSON-RPC endpoint polled for MessageSent events.
	RPCURL string
	// PackageID is the fractal-demail Move package id.
	PackageID string
	// Address is this node's Sui address (inbound recipient / outbound sender).
	Address string
	// IdentityKeyFile is a path to a base64 Ed25519 private key (32-byte seed
	// or 64-byte key). It is read at Start and never logged.
	IdentityKeyFile string
	// SponsorAddress is the gas sponsor Sui address for outbound sends.
	SponsorAddress string
	// GasCoin is the sponsor-owned gas coin object id used by outbound sends.
	GasCoin string
	// PollInterval controls inbound event polling. Zero uses the default (2s).
	PollInterval time.Duration
	// CursorFile stores processed Message object ids so historical event replay
	// on restart cannot re-execute old tasks or re-spend reply gas.
	CursorFile string
	// AllowedSenders is an allowlist of Sui addresses. Empty means deny all.
	AllowedSenders []string
	// Peers maps a recipient Sui address to its base64 Ed25519 public key.
	// A Sui address cannot be reversed into a public key, so outbound sends
	// require an explicit entry here.
	Peers map[string]string
}

// DemailChannel implements the Channel interface over fractal-demail:
// on-chain encrypted agent mail on Sui.
//
// Inbound: a fractal-demail listener polls MessageSent events, decrypts and
// sanitizes each message, and this channel hands it to the shared
// IncomingMessageHandler (same pathway as telegram/discord/imessage). The
// sender identity and chat id are both the sender's Sui address.
//
// Security posture: AllowedSenders is an explicit allowlist; when it is empty
// every sender is denied, matching the deny-all default of the other gateway
// channels.
type DemailChannel struct {
	rpcURL          string
	packageID       string
	address         string
	identityKeyFile string
	sponsorAddress  string
	gasCoin         string
	cursorFile      string
	pollInterval    time.Duration

	allowlist DemailAllowlist
	// peers maps normalized recipient address -> Ed25519 public key.
	peers map[string]ed25519.PublicKey

	handler IncomingMessageHandler

	// execFn runs an external command; injectable so tests can mock the sui CLI.
	execFn func(ctx context.Context, name string, args ...string) ([]byte, error)
	// runListenerFn runs the inbound loop until ctx is cancelled; injectable so
	// tests can feed fake events without a network. Nil means the real
	// fractal-demail listener is built at Start.
	runListenerFn func(ctx context.Context) error
	// nowFn provides plaintext timestamps; injectable for deterministic tests.
	nowFn func() time.Time

	runningMu sync.RWMutex
	running   bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	processedMu       sync.Mutex
	processedMessages map[string]struct{}

	telemetryMu  sync.RWMutex
	lastActivity time.Time
	lastError    time.Time
}

// NewDemailChannel validates options and returns a demail channel.
func NewDemailChannel(opts DemailOptions) (*DemailChannel, error) {
	if strings.TrimSpace(opts.RPCURL) == "" {
		return nil, errors.New("demail rpcUrl is required")
	}
	if strings.TrimSpace(opts.PackageID) == "" {
		return nil, errors.New("demail packageId is required")
	}
	address, err := normalizeDemailAddress(opts.Address)
	if err != nil {
		return nil, fmt.Errorf("demail address: %w", err)
	}

	sponsor := strings.TrimSpace(opts.SponsorAddress)
	if sponsor != "" {
		if sponsor, err = normalizeDemailAddress(sponsor); err != nil {
			return nil, fmt.Errorf("demail sponsorAddress: %w", err)
		}
	}

	gasCoin := strings.TrimSpace(opts.GasCoin)
	if gasCoin != "" {
		// Object ids share the Sui address format; reject anything else
		// before it can ever reach a CLI argv.
		if gasCoin, err = normalizeDemailAddress(gasCoin); err != nil {
			return nil, fmt.Errorf("demail gasCoin: %w", err)
		}
	}

	allowlist, err := NewDemailAllowlist(opts.AllowedSenders)
	if err != nil {
		return nil, err
	}

	cursorFile := strings.TrimSpace(opts.CursorFile)
	processedMessages, err := loadDemailProcessedMessages(cursorFile)
	if err != nil {
		return nil, fmt.Errorf("demail cursorFile: %w", err)
	}

	peers := make(map[string]ed25519.PublicKey, len(opts.Peers))
	for addr, pubB64 := range opts.Peers {
		normalized, err := normalizeDemailAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("demail peers[%s]: %w", addr, err)
		}
		pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
		if err != nil {
			return nil, fmt.Errorf("demail peers[%s]: invalid base64 public key", addr)
		}
		if len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("demail peers[%s]: public key must be %d bytes, got %d", addr, ed25519.PublicKeySize, len(pub))
		}
		peers[normalized] = ed25519.PublicKey(pub)
	}

	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultDemailPollInterval
	}

	channel := &DemailChannel{
		rpcURL:            strings.TrimSpace(opts.RPCURL),
		packageID:         strings.TrimSpace(opts.PackageID),
		address:           address,
		identityKeyFile:   strings.TrimSpace(opts.IdentityKeyFile),
		sponsorAddress:    sponsor,
		gasCoin:           gasCoin,
		pollInterval:      pollInterval,
		cursorFile:        cursorFile,
		allowlist:         allowlist,
		peers:             peers,
		processedMessages: processedMessages,
		nowFn:             time.Now,
	}
	channel.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		if err != nil {
			// Surface the CLI's own diagnostics (stderr is in CombinedOutput);
			// cap the tail so errors stay loggable.
			tail := lastNonEmptyLine(string(output))
			if len(tail) > 300 {
				tail = tail[:300]
			}
			return output, fmt.Errorf("%w: %s", err, tail)
		}
		return output, nil
	}
	return channel, nil
}

func (b *DemailChannel) Name() string {
	return "demail"
}

func (b *DemailChannel) SetHandler(handler IncomingMessageHandler) {
	b.handler = handler
}

func (b *DemailChannel) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

func (b *DemailChannel) setRunning(running bool) {
	b.runningMu.Lock()
	b.running = running
	b.runningMu.Unlock()
}

// LastActivity reports the last successful send or inbound handling time.
func (b *DemailChannel) LastActivity() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastActivity
}

// LastError reports the last channel error time.
func (b *DemailChannel) LastError() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastError
}

func (b *DemailChannel) markActivity() {
	b.telemetryMu.Lock()
	b.lastActivity = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *DemailChannel) markError() {
	b.telemetryMu.Lock()
	b.lastError = time.Now().UTC()
	b.telemetryMu.Unlock()
}

// Start builds the inbound listener and runs it until Stop or ctx cancel.
func (b *DemailChannel) Start(ctx context.Context) error {
	if b.IsRunning() {
		return errors.New("demail channel already running")
	}
	b.ctx, b.cancel = context.WithCancel(ctx)

	runFn := b.runListenerFn
	if runFn == nil {
		identityKey, err := loadDemailIdentityKey(b.identityKeyFile)
		if err != nil {
			b.cancel()
			return err
		}
		listenerCursorFile := ""
		if b.cursorFile != "" {
			// The journal file stores processed Message ids; the listener keeps
			// its RPC poll cursor in a sibling file so a restart resumes from
			// the last processed event instead of rescanning history.
			listenerCursorFile = b.cursorFile + ".cursor"
		}
		inbound, err := listener.New(listener.Config{
			RPCURL:       b.rpcURL,
			PackageID:    b.packageID,
			Recipient:    b.address,
			IdentityKey:  identityKey,
			PollInterval: b.pollInterval,
			CursorFile:   listenerCursorFile,
		}, func(messageID string, msg *schema.Plaintext) {
			b.handleInbound(messageID, msg)
		})
		if err != nil {
			b.cancel()
			return fmt.Errorf("demail listener init failed: %w", err)
		}
		runFn = inbound.Run
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		if err := runFn(b.ctx); err != nil && b.ctx.Err() == nil {
			log.Printf("demail listener exited: %v", err)
			b.markError()
			// The inbound loop is dead; reflect reality for health checks.
			b.setRunning(false)
		}
	}()

	b.setRunning(true)
	return nil
}

// Stop cancels the listener context and waits for the poll loop to exit.
func (b *DemailChannel) Stop(ctx context.Context) error {
	_ = ctx
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.setRunning(false)
	return nil
}

// IsAllowed reports whether the sender Sui address is on the allowlist.
// An empty allowlist denies all senders.
func (b *DemailChannel) IsAllowed(senderID string) bool {
	return b.allowlist.Allowed(senderID)
}

// handleInbound delivers one decrypted message to the shared inbound handler.
// Input is untrusted even after schema sanitization; it must never panic.
func (b *DemailChannel) handleInbound(messageID string, msg *schema.Plaintext) {
	messageID = strings.TrimSpace(messageID)
	if messageID != "" && b.isProcessed(messageID) {
		return
	}
	if msg == nil {
		return
	}
	from := strings.ToLower(strings.TrimSpace(msg.From))
	if from == "" {
		return
	}
	if !b.allowlist.Allowed(from) {
		log.Printf("demail: dropping message %s from non-allowlisted sender %s", messageID, from)
		return
	}
	if b.handler == nil {
		return
	}

	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	reply, err := b.handler.HandleIncoming(ctx, b.toProtocolMessage(messageID, from, msg))
	if err != nil {
		b.markError()
		log.Printf("demail inbound handler failed for message %s: %v", messageID, err)
		return
	}
	b.markActivity()

	defer func() {
		if messageID != "" {
			b.markProcessed(messageID)
		}
	}()

	// Loop guard: only "task" messages get an automatic on-chain reply.
	// Outbound sends are stamped type "reply", so two mutually-allowlisted
	// gateways cannot ping-pong sponsored transactions forever.
	if msg.Type != schema.TypeTask {
		return
	}
	if trimmed := strings.TrimSpace(reply); trimmed != "" {
		if _, err := b.Send(ctx, OutboundMessage{To: from, Text: trimmed}); err != nil {
			log.Printf("demail reply send failed: %v", err)
		}
	}
}

func loadDemailProcessedMessages(path string) (map[string]struct{}, error) {
	processed := make(map[string]struct{})
	if path == "" {
		return processed, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return processed, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		processed[line] = struct{}{}
	}
	return processed, nil
}

func (b *DemailChannel) isProcessed(messageID string) bool {
	b.processedMu.Lock()
	defer b.processedMu.Unlock()
	_, ok := b.processedMessages[messageID]
	return ok
}

func (b *DemailChannel) markProcessed(messageID string) {
	b.processedMu.Lock()
	if _, ok := b.processedMessages[messageID]; ok {
		b.processedMu.Unlock()
		return
	}
	b.processedMessages[messageID] = struct{}{}
	b.processedMu.Unlock()

	if b.cursorFile == "" {
		return
	}
	dir := filepath.Dir(b.cursorFile)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Printf("demail cursor mkdir failed: %v", err)
			return
		}
	}
	f, err := os.OpenFile(b.cursorFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("demail cursor open failed: %v", err)
		return
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, messageID); err != nil {
		log.Printf("demail cursor write failed: %v", err)
	}
}

func (b *DemailChannel) toProtocolMessage(messageID, from string, msg *schema.Plaintext) *protocol.Message {
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data: map[string]interface{}{
			"channel":    "demail",
			"text":       msg.Body,
			"agent":      "",
			"user_id":    from,
			"chat_id":    from,
			"sender":     from,
			"message_id": messageID,
			"subject":    msg.Subject,
			"msg_type":   msg.Type,
			"reply_to":   msg.ReplyTo,
			"timestamp":  msg.TS,
			"chatType":   "dm",
		},
	}
}

// Send encrypts msg.Text to the recipient's registered Ed25519 key and
// submits a sponsored demail::send_inline transaction through the sui CLI.
// The transaction digest is returned in SendResult.MessageTS (documented
// choice: MessageTS is the closest analogue to a per-message id; ChannelID
// carries the recipient address).
func (b *DemailChannel) Send(ctx context.Context, msg OutboundMessage) (*SendResult, error) {
	recipient, err := normalizeDemailAddress(msg.To)
	if err != nil {
		return nil, fmt.Errorf("demail recipient: %w", err)
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil, errors.New("demail text is required")
	}
	if b.sponsorAddress == "" {
		return nil, errors.New("demail sponsorAddress is required for outbound sends")
	}
	if b.gasCoin == "" {
		return nil, errors.New("demail gasCoin is required for outbound sends")
	}
	if b.execFn == nil {
		return nil, errors.New("demail command runner not configured")
	}

	recipientPub, ok := b.peers[recipient]
	if !ok {
		return nil, fmt.Errorf("demail: no peer public key configured for %s (add it to channels.demail.peers)", recipient)
	}

	senderRaw, err := demailAddressBytes(b.address)
	if err != nil {
		return nil, fmt.Errorf("demail sender address: %w", err)
	}
	recipientRaw, err := demailAddressBytes(recipient)
	if err != nil {
		return nil, fmt.Errorf("demail recipient address: %w", err)
	}

	plaintext, err := json.Marshal(schema.Plaintext{
		Type: schema.TypeReply,
		From: b.address,
		To:   recipient,
		Body: text,
		TS:   b.nowFn().UnixMilli(),
	})
	if err != nil {
		return nil, fmt.Errorf("demail plaintext encode failed: %w", err)
	}

	env, err := envelope.Seal(recipientPub, senderRaw, recipientRaw, plaintext)
	if err != nil {
		return nil, fmt.Errorf("demail envelope seal failed: %w", err)
	}
	envJSON, err := env.Marshal()
	if err != nil {
		return nil, fmt.Errorf("demail envelope encode failed: %w", err)
	}
	payloadB64 := base64.StdEncoding.EncodeToString(envJSON)

	txBytes, err := b.serializeUnsignedTx(ctx, recipient, payloadB64)
	if err != nil {
		b.markError()
		return nil, err
	}

	transport := &demailCLITransport{execFn: b.execFn}
	relay, err := gasstation.New(gasstation.Route{Sender: b.address, GasSponsor: b.sponsorAddress}, transport)
	if err != nil {
		return nil, fmt.Errorf("demail relay init failed: %w", err)
	}

	request, err := relay.Sponsor(ctx, []byte(txBytes), b.suiSigner(b.address), b.suiSigner(b.sponsorAddress))
	if err != nil {
		b.markError()
		return nil, fmt.Errorf("demail sponsor signing failed: %w", err)
	}
	if err := relay.Relay(ctx, request); err != nil {
		b.markError()
		return nil, fmt.Errorf("demail relay failed: %w", err)
	}

	b.markActivity()
	return &SendResult{
		ChannelName: "demail",
		ChannelID:   recipient,
		MessageTS:   transport.digest,
	}, nil
}

// serializeUnsignedTx builds the sponsored PTB and returns base64 tx bytes.
func (b *DemailChannel) serializeUnsignedTx(ctx context.Context, recipient, payloadB64 string) (string, error) {
	args := []string{
		"client", "ptb",
		"--sender", "@" + b.address,
		"--move-call", b.packageID + "::demail::send_inline",
		"@" + recipient,
		`"` + payloadB64 + `"`,
		"@" + demailClockObjectID,
		"--gas-sponsor", "@" + b.sponsorAddress,
		"--gas-coin", "@" + b.gasCoin,
		"--gas-budget", demailGasBudget,
		"--serialize-unsigned-transaction",
	}
	output, err := b.execFn(ctx, "sui", args...)
	if err != nil {
		return "", fmt.Errorf("demail tx serialization failed: %w", err)
	}
	txBytes := lastNonEmptyLine(string(output))
	if txBytes == "" {
		return "", errors.New("demail tx serialization returned no tx bytes")
	}
	return txBytes, nil
}

// suiSigner signs base64 tx bytes with the sui keytool for the given address.
// The returned bytes are the base64 .suiSignature string.
func (b *DemailChannel) suiSigner(address string) gasstation.Signer {
	return func(ctx context.Context, txBytes []byte) ([]byte, error) {
		output, err := b.execFn(ctx, "sui", "keytool", "sign", "--address", address, "--data", string(txBytes), "--json")
		if err != nil {
			return nil, fmt.Errorf("sui keytool sign failed for %s: %w", address, err)
		}
		var result struct {
			SuiSignature string `json:"suiSignature"`
		}
		if err := unmarshalDemailCLIJSON(output, &result); err != nil {
			return nil, fmt.Errorf("sui keytool sign returned invalid json for %s: %w", address, err)
		}
		signature := strings.TrimSpace(result.SuiSignature)
		if signature == "" {
			return nil, fmt.Errorf("sui keytool sign returned empty suiSignature for %s", address)
		}
		return []byte(signature), nil
	}
}

// demailCLITransport executes the dual-signed transaction via the sui CLI,
// keeping the gasstation route/signature invariants on the outbound path.
type demailCLITransport struct {
	execFn func(ctx context.Context, name string, args ...string) ([]byte, error)
	digest string
}

func (t *demailCLITransport) Relay(ctx context.Context, req gasstation.RelayRequest) error {
	output, err := t.execFn(ctx, "sui",
		"client", "execute-signed-tx",
		"--tx-bytes", string(req.UnsignedTx),
		"--signatures", string(req.SenderSignature),
		"--signatures", string(req.SponsorSignature),
		"--json",
	)
	if err != nil {
		return fmt.Errorf("sui execute-signed-tx failed: %w", err)
	}
	var result struct {
		Digest string `json:"digest"`
	}
	if err := unmarshalDemailCLIJSON(output, &result); err != nil {
		return fmt.Errorf("sui execute-signed-tx returned invalid json: %w", err)
	}
	if strings.TrimSpace(result.Digest) == "" {
		return errors.New("sui execute-signed-tx returned no digest")
	}
	t.digest = strings.TrimSpace(result.Digest)
	return nil
}

// DemailAllowlist is an allowlist of normalized Sui addresses.
// Empty means deny all (gateway security posture).
type DemailAllowlist struct {
	configured bool
	allowed    map[string]struct{}
}

// NewDemailAllowlist normalizes and validates allowlist entries.
func NewDemailAllowlist(values []string) (DemailAllowlist, error) {
	allowed := make(map[string]struct{})
	for idx, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized, err := normalizeDemailAddress(trimmed)
		if err != nil {
			return DemailAllowlist{}, fmt.Errorf("demail allowedSenders[%d]: %w", idx, err)
		}
		allowed[normalized] = struct{}{}
	}
	return DemailAllowlist{configured: len(allowed) > 0, allowed: allowed}, nil
}

// Allowed reports whether the address is allowlisted. Deny all when empty.
func (a DemailAllowlist) Allowed(address string) bool {
	if !a.configured {
		return false
	}
	normalized, err := normalizeDemailAddress(address)
	if err != nil {
		return false
	}
	_, ok := a.allowed[normalized]
	return ok
}

// loadDemailIdentityKey reads a base64 Ed25519 private key (32-byte seed or
// 64-byte key) from path. Key material is never logged.
func loadDemailIdentityKey(path string) (ed25519.PrivateKey, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("demail identityKeyFile is required")
	}
	expanded, err := expandHomePath(path)
	if err != nil {
		return nil, fmt.Errorf("demail identityKeyFile: %w", err)
	}
	data, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("demail identityKeyFile: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, errors.New("demail identityKeyFile: content must be base64")
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	default:
		return nil, fmt.Errorf("demail identityKeyFile: key must be %d-byte seed or %d-byte key, got %d bytes", ed25519.SeedSize, ed25519.PrivateKeySize, len(raw))
	}
}

// normalizeDemailAddress lowercases and validates a 0x-prefixed 32-byte
// Sui address.
func normalizeDemailAddress(address string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(address))
	if _, err := demailAddressBytes(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

// demailAddressBytes converts a 0x-prefixed Sui address to raw 32 bytes.
func demailAddressBytes(address string) ([]byte, error) {
	trimmed := strings.ToLower(strings.TrimSpace(address))
	if !strings.HasPrefix(trimmed, "0x") {
		return nil, fmt.Errorf("invalid sui address %q", address)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(trimmed, "0x"))
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("invalid sui address %q", address)
	}
	return raw, nil
}

// unmarshalDemailCLIJSON parses JSON from CLI output, tolerating any
// non-JSON preamble the sui CLI prints before the JSON document.
func unmarshalDemailCLIJSON(output []byte, out interface{}) error {
	start := -1
	for i, c := range output {
		if c == '{' || c == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return errors.New("no json document in output")
	}
	return json.Unmarshal(output[start:], out)
}

func lastNonEmptyLine(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
