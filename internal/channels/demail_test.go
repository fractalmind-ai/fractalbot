package channels

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractal-demail/client-go/envelope"
	"github.com/fractalmind-ai/fractal-demail/client-go/schema"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

var (
	demailTestAddress   = "0x" + strings.Repeat("11", 32)
	demailTestPeer      = "0x" + strings.Repeat("22", 32)
	demailTestSponsor   = "0x" + strings.Repeat("33", 32)
	demailTestGasCoin   = "0x" + strings.Repeat("44", 32)
	demailTestPackageID = "0x" + strings.Repeat("55", 32)
)

type fakeDemailHandler struct {
	reply string
	err   error
	calls int
	msgs  []*protocol.Message
}

func (f *fakeDemailHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	_ = ctx
	f.calls++
	f.msgs = append(f.msgs, msg)
	return f.reply, f.err
}

func newTestDemailChannel(t *testing.T, opts DemailOptions) *DemailChannel {
	t.Helper()
	if opts.RPCURL == "" {
		opts.RPCURL = "http://127.0.0.1:9000"
	}
	if opts.PackageID == "" {
		opts.PackageID = demailTestPackageID
	}
	if opts.Address == "" {
		opts.Address = demailTestAddress
	}
	channel, err := NewDemailChannel(opts)
	if err != nil {
		t.Fatalf("NewDemailChannel: %v", err)
	}
	return channel
}

func TestNewDemailChannelValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    DemailOptions
		wantErr string
	}{
		{
			name:    "missing rpc url",
			opts:    DemailOptions{PackageID: demailTestPackageID, Address: demailTestAddress},
			wantErr: "rpcUrl",
		},
		{
			name:    "missing package id",
			opts:    DemailOptions{RPCURL: "http://127.0.0.1:9000", Address: demailTestAddress},
			wantErr: "packageId",
		},
		{
			name:    "invalid address",
			opts:    DemailOptions{RPCURL: "http://127.0.0.1:9000", PackageID: demailTestPackageID, Address: "0x123"},
			wantErr: "address",
		},
		{
			name: "invalid sponsor address",
			opts: DemailOptions{
				RPCURL: "http://127.0.0.1:9000", PackageID: demailTestPackageID, Address: demailTestAddress,
				SponsorAddress: "not-an-address",
			},
			wantErr: "sponsorAddress",
		},
		{
			name: "invalid allowlist entry",
			opts: DemailOptions{
				RPCURL: "http://127.0.0.1:9000", PackageID: demailTestPackageID, Address: demailTestAddress,
				AllowedSenders: []string{"0xzz"},
			},
			wantErr: "allowedSenders[0]",
		},
		{
			name: "invalid peer address",
			opts: DemailOptions{
				RPCURL: "http://127.0.0.1:9000", PackageID: demailTestPackageID, Address: demailTestAddress,
				Peers: map[string]string{"bogus": base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))},
			},
			wantErr: "peers[bogus]",
		},
		{
			name: "invalid peer public key encoding",
			opts: DemailOptions{
				RPCURL: "http://127.0.0.1:9000", PackageID: demailTestPackageID, Address: demailTestAddress,
				Peers: map[string]string{demailTestPeer: "%%%not-base64%%%"},
			},
			wantErr: "invalid base64 public key",
		},
		{
			name: "invalid peer public key length",
			opts: DemailOptions{
				RPCURL: "http://127.0.0.1:9000", PackageID: demailTestPackageID, Address: demailTestAddress,
				Peers: map[string]string{demailTestPeer: base64.StdEncoding.EncodeToString(make([]byte, 16))},
			},
			wantErr: "public key must be 32 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDemailChannel(tt.opts)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestDemailAllowlistDeniesAllWhenEmpty(t *testing.T) {
	channel := newTestDemailChannel(t, DemailOptions{})
	if channel.IsAllowed(demailTestPeer) {
		t.Fatal("empty allowlist must deny all senders")
	}
}

func TestDemailIsAllowedNormalizesCase(t *testing.T) {
	channel := newTestDemailChannel(t, DemailOptions{AllowedSenders: []string{demailTestPeer}})
	if !channel.IsAllowed("0x" + strings.ToUpper(strings.Repeat("22", 32))) {
		t.Fatal("allowlist must be case-insensitive")
	}
	if channel.IsAllowed(demailTestSponsor) {
		t.Fatal("unlisted sender must be denied")
	}
	if channel.IsAllowed("garbage") {
		t.Fatal("malformed sender must be denied")
	}
}

func TestDemailInboundDelivery(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		wantHandled bool
	}{
		{name: "allowed sender delivers", from: demailTestPeer, wantHandled: true},
		{name: "unknown sender blocked", from: demailTestSponsor, wantHandled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &fakeDemailHandler{}
			channel := newTestDemailChannel(t, DemailOptions{AllowedSenders: []string{demailTestPeer}})
			channel.SetHandler(handler)

			delivered := make(chan struct{})
			channel.runListenerFn = func(ctx context.Context) error {
				channel.handleInbound("0xmsg1", &schema.Plaintext{
					Type: schema.TypeTask,
					From: tt.from,
					To:   demailTestAddress,
					Body: "run the tests",
					TS:   1234,
				})
				close(delivered)
				<-ctx.Done()
				return ctx.Err()
			}

			if err := channel.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			<-delivered
			if err := channel.Stop(context.Background()); err != nil {
				t.Fatalf("Stop: %v", err)
			}

			if !tt.wantHandled {
				if handler.calls != 0 {
					t.Fatalf("expected handler not to be called, got %d calls", handler.calls)
				}
				return
			}
			if handler.calls != 1 {
				t.Fatalf("expected 1 handler call, got %d", handler.calls)
			}
			data, ok := handler.msgs[0].Data.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map data, got %T", handler.msgs[0].Data)
			}
			if data["channel"] != "demail" {
				t.Fatalf("expected channel demail, got %v", data["channel"])
			}
			if data["user_id"] != tt.from {
				t.Fatalf("expected user_id %s, got %v", tt.from, data["user_id"])
			}
			if data["chat_id"] != tt.from {
				t.Fatalf("expected chat_id %s, got %v", tt.from, data["chat_id"])
			}
			if data["text"] != "run the tests" {
				t.Fatalf("expected body text, got %v", data["text"])
			}
			if data["message_id"] != "0xmsg1" {
				t.Fatalf("expected message_id 0xmsg1, got %v", data["message_id"])
			}
		})
	}
}

func TestDemailInboundSurvivesMalformedInput(t *testing.T) {
	handler := &fakeDemailHandler{}
	channel := newTestDemailChannel(t, DemailOptions{AllowedSenders: []string{demailTestPeer}})
	channel.SetHandler(handler)

	// Must not panic on nil or partially-populated plaintexts.
	channel.handleInbound("0xmsg1", nil)
	channel.handleInbound("0xmsg2", &schema.Plaintext{})
	channel.handleInbound("0xmsg3", &schema.Plaintext{From: "not-an-address"})

	if handler.calls != 0 {
		t.Fatalf("expected no handler calls for malformed input, got %d", handler.calls)
	}
}

func TestDemailStopTerminatesListener(t *testing.T) {
	channel := newTestDemailChannel(t, DemailOptions{})
	stopped := make(chan struct{})
	channel.runListenerFn = func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}

	if err := channel.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !channel.IsRunning() {
		t.Fatal("expected channel to be running after Start")
	}

	if err := channel.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not terminate after Stop")
	}
	if channel.IsRunning() {
		t.Fatal("expected channel to not be running after Stop")
	}
}

func TestDemailStartRequiresIdentityKey(t *testing.T) {
	channel := newTestDemailChannel(t, DemailOptions{})
	err := channel.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to fail without identityKeyFile")
	}
	if !strings.Contains(err.Error(), "identityKeyFile") {
		t.Fatalf("expected identityKeyFile error, got %v", err)
	}
	if channel.IsRunning() {
		t.Fatal("channel must not report running after failed Start")
	}
}

func TestLoadDemailIdentityKey(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	fullKey := ed25519.NewKeyFromSeed(seed)

	writeKey := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "identity.key")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatalf("write key: %v", err)
		}
		return path
	}

	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "32-byte seed", content: base64.StdEncoding.EncodeToString(seed)},
		{name: "64-byte key", content: base64.StdEncoding.EncodeToString(fullKey)},
		{name: "seed with trailing newline", content: base64.StdEncoding.EncodeToString(seed) + "\n"},
		{name: "not base64", content: "%%%", wantErr: "must be base64"},
		{name: "wrong length", content: base64.StdEncoding.EncodeToString(make([]byte, 16)), wantErr: "32-byte seed or 64-byte key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := loadDemailIdentityKey(writeKey(t, tt.content))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadDemailIdentityKey: %v", err)
			}
			if !key.Equal(fullKey) {
				t.Fatal("loaded key does not match expected key")
			}
		})
	}

	if _, err := loadDemailIdentityKey(""); err == nil {
		t.Fatal("expected error for empty path")
	}
	if _, err := loadDemailIdentityKey(filepath.Join(t.TempDir(), "missing.key")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

type demailExecCall struct {
	name string
	args []string
}

func TestDemailSendCLISequence(t *testing.T) {
	recipientPub, recipientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate recipient key: %v", err)
	}

	channel := newTestDemailChannel(t, DemailOptions{
		SponsorAddress: demailTestSponsor,
		GasCoin:        demailTestGasCoin,
		Peers: map[string]string{
			demailTestPeer: base64.StdEncoding.EncodeToString(recipientPub),
		},
	})
	channel.nowFn = func() time.Time { return time.UnixMilli(1750000000000) }

	const txBytes = "dGVzdC10eC1ieXRlcw=="
	var calls []demailExecCall
	channel.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		_ = ctx
		calls = append(calls, demailExecCall{name: name, args: append([]string{}, args...)})
		switch {
		case len(args) > 1 && args[0] == "client" && args[1] == "ptb":
			return []byte(txBytes + "\n"), nil
		case len(args) > 1 && args[0] == "keytool" && args[1] == "sign":
			if args[3] == demailTestAddress {
				return []byte(`{"suiSignature":"sig-sender"}`), nil
			}
			return []byte(`{"suiSignature":"sig-sponsor"}`), nil
		case len(args) > 1 && args[0] == "client" && args[1] == "execute-signed-tx":
			return []byte(`{"digest":"DIGEST123"}`), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}

	result, err := channel.Send(context.Background(), OutboundMessage{To: demailTestPeer, Text: "hello peer"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.MessageTS != "DIGEST123" {
		t.Fatalf("expected digest in MessageTS, got %q", result.MessageTS)
	}
	if result.ChannelID != demailTestPeer {
		t.Fatalf("expected recipient in ChannelID, got %q", result.ChannelID)
	}
	if result.ChannelName != "demail" {
		t.Fatalf("expected ChannelName demail, got %q", result.ChannelName)
	}

	if len(calls) != 4 {
		t.Fatalf("expected 4 CLI calls, got %d", len(calls))
	}
	for i, call := range calls {
		if call.name != "sui" {
			t.Fatalf("call %d: expected sui binary, got %q", i, call.name)
		}
	}

	// Call 1: serialize the sponsored PTB. The envelope payload is random
	// (ephemeral key + nonce), so capture it and verify separately.
	ptbArgs := calls[0].args
	if len(ptbArgs) != 16 {
		t.Fatalf("expected 16 ptb args, got %d: %v", len(ptbArgs), ptbArgs)
	}
	payloadArg := ptbArgs[7]
	expectedPtb := []string{
		"client", "ptb",
		"--sender", "@" + demailTestAddress,
		"--move-call", demailTestPackageID + "::demail::send_inline",
		"@" + demailTestPeer,
		payloadArg,
		"@0x6",
		"--gas-sponsor", "@" + demailTestSponsor,
		"--gas-coin", "@" + demailTestGasCoin,
		"--gas-budget", "10000000",
		"--serialize-unsigned-transaction",
	}
	if !reflect.DeepEqual(ptbArgs, expectedPtb) {
		t.Fatalf("unexpected ptb args:\n got %v\nwant %v", ptbArgs, expectedPtb)
	}

	// Calls 2+3: dual signatures over the identical tx bytes.
	expectedSenderSign := []string{"keytool", "sign", "--address", demailTestAddress, "--data", txBytes, "--json"}
	if !reflect.DeepEqual(calls[1].args, expectedSenderSign) {
		t.Fatalf("unexpected sender sign args:\n got %v\nwant %v", calls[1].args, expectedSenderSign)
	}
	expectedSponsorSign := []string{"keytool", "sign", "--address", demailTestSponsor, "--data", txBytes, "--json"}
	if !reflect.DeepEqual(calls[2].args, expectedSponsorSign) {
		t.Fatalf("unexpected sponsor sign args:\n got %v\nwant %v", calls[2].args, expectedSponsorSign)
	}

	// Call 4: execution with both signatures in sender, sponsor order.
	expectedExecute := []string{
		"client", "execute-signed-tx",
		"--tx-bytes", txBytes,
		"--signatures", "sig-sender",
		"--signatures", "sig-sponsor",
		"--json",
	}
	if !reflect.DeepEqual(calls[3].args, expectedExecute) {
		t.Fatalf("unexpected execute args:\n got %v\nwant %v", calls[3].args, expectedExecute)
	}

	// The payload must be a quoted base64 envelope sealed to the recipient's
	// Ed25519 key with the on-chain route as AAD.
	if !strings.HasPrefix(payloadArg, `"`) || !strings.HasSuffix(payloadArg, `"`) {
		t.Fatalf("expected quoted payload arg, got %q", payloadArg)
	}
	envJSON, err := base64.StdEncoding.DecodeString(strings.Trim(payloadArg, `"`))
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	env, err := envelope.Unmarshal(envJSON)
	if err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	senderRaw, err := demailAddressBytes(demailTestAddress)
	if err != nil {
		t.Fatalf("sender address bytes: %v", err)
	}
	recipientRaw, err := demailAddressBytes(demailTestPeer)
	if err != nil {
		t.Fatalf("recipient address bytes: %v", err)
	}
	plaintext, err := envelope.Open(recipientPriv, senderRaw, recipientRaw, env)
	if err != nil {
		t.Fatalf("envelope must decrypt with the recipient key: %v", err)
	}
	parsed, err := schema.Parse(plaintext, demailTestAddress, demailTestPeer)
	if err != nil {
		t.Fatalf("plaintext must satisfy the demail schema: %v", err)
	}
	if parsed.Type != schema.TypeReply {
		t.Fatalf("expected type reply, got %q", parsed.Type)
	}
	if parsed.Body != "hello peer" {
		t.Fatalf("expected body 'hello peer', got %q", parsed.Body)
	}
	if parsed.TS != 1750000000000 {
		t.Fatalf("expected ts 1750000000000, got %d", parsed.TS)
	}
}

func TestDemailSendFailures(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	peerKey := base64.StdEncoding.EncodeToString(pub)

	tests := []struct {
		name    string
		opts    DemailOptions
		msg     OutboundMessage
		wantErr string
	}{
		{
			name:    "unknown peer",
			opts:    DemailOptions{SponsorAddress: demailTestSponsor, GasCoin: demailTestGasCoin},
			msg:     OutboundMessage{To: demailTestPeer, Text: "hi"},
			wantErr: "no peer public key configured",
		},
		{
			name:    "invalid recipient",
			opts:    DemailOptions{SponsorAddress: demailTestSponsor, GasCoin: demailTestGasCoin},
			msg:     OutboundMessage{To: "0x123", Text: "hi"},
			wantErr: "recipient",
		},
		{
			name:    "empty text",
			opts:    DemailOptions{SponsorAddress: demailTestSponsor, GasCoin: demailTestGasCoin},
			msg:     OutboundMessage{To: demailTestPeer, Text: "  "},
			wantErr: "text is required",
		},
		{
			name:    "missing sponsor",
			opts:    DemailOptions{GasCoin: demailTestGasCoin, Peers: map[string]string{demailTestPeer: peerKey}},
			msg:     OutboundMessage{To: demailTestPeer, Text: "hi"},
			wantErr: "sponsorAddress is required",
		},
		{
			name:    "missing gas coin",
			opts:    DemailOptions{SponsorAddress: demailTestSponsor, Peers: map[string]string{demailTestPeer: peerKey}},
			msg:     OutboundMessage{To: demailTestPeer, Text: "hi"},
			wantErr: "gasCoin is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := newTestDemailChannel(t, tt.opts)
			channel.execFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
				t.Fatalf("no CLI command should run, got %s %v", name, args)
				return nil, nil
			}
			_, err := channel.Send(context.Background(), tt.msg)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
