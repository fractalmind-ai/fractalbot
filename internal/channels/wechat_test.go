package channels

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type fakeWeChatHandler struct {
	reply string
	err   error
	last  *protocol.Message
}

func (f *fakeWeChatHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	_ = ctx
	f.last = msg
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

func TestWeChatCallbackGETHandshakeEchoesEchostrWhenSignatureValid(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigureCallback("", "/wechat/callback", "token-123", "aes-key-123")

	echostr := "hello"
	sig := computeWeChatSignature("token-123", "1", "2", echostr)
	req := httptest.NewRequest(http.MethodGet, "/wechat/callback?msg_signature="+sig+"&timestamp=1&nonce=2&echostr="+echostr, nil)
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); body != echostr {
		t.Fatalf("body=%q want %q", body, echostr)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("unexpected content type: %s", got)
	}
}

func TestWeChatCallbackGETRejectsInvalidSignature(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigureCallback("", "/wechat/callback", "token-123", "")

	req := httptest.NewRequest(http.MethodGet, "/wechat/callback?signature=bad&timestamp=1&nonce=2&echostr=hello", nil)
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), `"error":"invalid callback signature"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatCallbackPOSTParsesXMLPayload(t *testing.T) {
	bot, err := NewWeChatBot("mp_service")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}

	xmlBody := `<xml><ToUserName><![CDATA[toUser]]></ToUserName><FromUserName><![CDATA[fromUser]]></FromUserName><CreateTime>1348831860</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[this is a test]]></Content><MsgId>1234567890123456</MsgId></xml>`
	req := httptest.NewRequest(http.MethodPost, "/wechat/callback", strings.NewReader(xmlBody))
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusNotImplemented)
	}
	body := rr.Body.String()
	for _, want := range []string{"\"provider\":\"mp_service\"", "\"MsgType\":\"text\"", "\"Content\":\"this is a test\"", "\"ToUserName\":\"toUser\"", "\"protocol_message\"", "\"channel\":\"wechat\"", "\"msg_type\":\"text\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body %s", want, body)
		}
	}
}

func TestWeChatCallbackRejectsUnsupportedMethod(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/wechat/callback", nil)
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "unsupported callback method") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatEnvelopeToProtocolMessageMapsEventFields(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}

	msg := bot.envelopeToProtocolMessage(&wechatCallbackEnvelope{
		ToUserName:   "toUser",
		FromUserName: "fromUser",
		MsgType:      "event",
		Event:        "subscribe",
		EventKey:     "qrscene_123",
		AgentID:      "1000001",
	})
	if msg == nil {
		t.Fatalf("expected protocol message")
	}
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %#v", msg.Data)
	}
	if data["channel"] != "wechat" || data["provider"] != "wecom" {
		t.Fatalf("unexpected base fields: %#v", data)
	}
	if data["event"] != "subscribe" || data["event_key"] != "qrscene_123" {
		t.Fatalf("unexpected event fields: %#v", data)
	}
	if data["agent_id"] != "1000001" {
		t.Fatalf("unexpected agent_id: %#v", data)
	}
}

func TestWeChatCallbackPOSTInvokesHandlerWithProtocolMessage(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	handler := &fakeWeChatHandler{reply: "agent-ack"}
	bot.SetHandler(handler)

	xmlBody := `<xml><ToUserName><![CDATA[toUser]]></ToUserName><FromUserName><![CDATA[fromUser]]></FromUserName><CreateTime>1348831860</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[this is a test]]></Content><MsgId>1234567890123456</MsgId></xml>`
	req := httptest.NewRequest(http.MethodPost, "/wechat/callback", strings.NewReader(xmlBody))
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if handler.last == nil {
		t.Fatalf("expected handler to receive protocol message")
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"handler_reply":"agent-ack"`) {
		t.Fatalf("expected handler reply in body: %s", body)
	}
}

func TestWeChatCallbackPOSTIncludesHandlerError(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.SetHandler(&fakeWeChatHandler{err: errors.New("boom")})

	xmlBody := `<xml><ToUserName><![CDATA[toUser]]></ToUserName><FromUserName><![CDATA[fromUser]]></FromUserName><CreateTime>1348831860</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[this is a test]]></Content><MsgId>1234567890123456</MsgId></xml>`
	req := httptest.NewRequest(http.MethodPost, "/wechat/callback", strings.NewReader(xmlBody))
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusNotImplemented)
	}
	if !strings.Contains(rr.Body.String(), `"handler_error":"boom"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatCallbackGETRequiresEchostr(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigureCallback("", "/wechat/callback", "token-123", "")

	sig := computeWeChatSignature("token-123", "1", "2", "hello")
	req := httptest.NewRequest(http.MethodGet, "/wechat/callback?msg_signature="+sig+"&timestamp=1&nonce=2", nil)
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), `"error":"missing echostr query parameter"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatCallbackGETRequiresSignatureWhenTokenConfigured(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigureCallback("", "/wechat/callback", "token-123", "")

	req := httptest.NewRequest(http.MethodGet, "/wechat/callback?timestamp=1&nonce=2&echostr=hello", nil)
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), `"error":"missing signature query parameter"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatCallbackGETRequiresTimestampWhenTokenConfigured(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigureCallback("", "/wechat/callback", "token-123", "")

	req := httptest.NewRequest(http.MethodGet, "/wechat/callback?signature=abc&nonce=2&echostr=hello", nil)
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), `"error":"missing timestamp query parameter"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatCallbackGETRequiresNonceWhenTokenConfigured(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigureCallback("", "/wechat/callback", "token-123", "")

	req := httptest.NewRequest(http.MethodGet, "/wechat/callback?signature=abc&timestamp=1&echostr=hello", nil)
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), `"error":"missing nonce query parameter"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatCallbackPOSTRequiresSignatureWhenTokenConfigured(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigureCallback("", "/wechat/callback", "token-123", "")

	xmlBody := `<xml><ToUserName><![CDATA[toUser]]></ToUserName><FromUserName><![CDATA[fromUser]]></FromUserName><CreateTime>1348831860</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[this is a test]]></Content><MsgId>1234567890123456</MsgId></xml>`
	req := httptest.NewRequest(http.MethodPost, "/wechat/callback", strings.NewReader(xmlBody))
	rr := httptest.NewRecorder()
	bot.handleCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), `"error":"missing signature query parameter"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWeChatPollingStateLoadMissingFile(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.pollingStateFile = filepath.Join(t.TempDir(), "wechat.state.json")

	if err := bot.loadPollingState(); err != nil {
		t.Fatalf("loadPollingState: %v", err)
	}
	if got := bot.getPollingCursor(); got != "" {
		t.Fatalf("polling cursor=%q want empty", got)
	}
}

func TestWeChatPollingStatePersistsCursor(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	path := filepath.Join(t.TempDir(), "state", "wechat.state.json")
	bot.pollingStateFile = path
	bot.setPollingCursor("cursor-42")

	if err := bot.persistPollingState(); err != nil {
		t.Fatalf("persistPollingState: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var state wechatPollingState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if state.NextCursor != "cursor-42" {
		t.Fatalf("NextCursor=%q want cursor-42", state.NextCursor)
	}
}

func TestWeChatPollingFetchUsesRealUpstreamAPI(t *testing.T) {
	var gotAuth string
	var gotAuthType string
	var gotAppID string
	var gotClientVersion string
	var gotUIN string
	var gotPayload wechatPollingGetUpdatesRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s want POST", r.Method)
		}
		if r.URL.Path != "/gateway/ilink/bot/getupdates" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotAuthType = r.Header.Get("AuthorizationType")
		gotAppID = r.Header.Get("iLink-App-Id")
		gotClientVersion = r.Header.Get("iLink-App-ClientVersion")
		gotUIN = r.Header.Get("X-WECHAT-UIN")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(wechatPollingResponse{
			Ret:           0,
			Errcode:       0,
			GetUpdatesBuf: "cursor-2",
			Msgs: []wechatPollingMessage{
				{
					MessageID:    101,
					ClientID:     "client-101",
					FromUserID:   "wx-user",
					ToUserID:     "wx-bot",
					SessionID:    "session-1",
					ContextToken: "context-1",
					ItemList: []wechatPollingMessageItem{
						{
							Type:     wechatMessageItemTypeText,
							TextItem: &wechatPollingTextItem{Text: "hello from ilink"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}
	bot.ConfigurePolling(server.URL+"/gateway", "bot-token", "", 1)

	msgs, nextCursor, err := bot.fetchPollingUpdates(context.Background(), "cursor-1")
	if err != nil {
		t.Fatalf("fetchPollingUpdates: %v", err)
	}
	if nextCursor != "cursor-2" {
		t.Fatalf("nextCursor=%q want cursor-2", nextCursor)
	}
	if gotPayload.GetUpdatesBuf != "cursor-1" {
		t.Fatalf("get_updates_buf=%q want cursor-1", gotPayload.GetUpdatesBuf)
	}
	if gotPayload.BaseInfo.ChannelVersion != wechatIlinkChannelVersion {
		t.Fatalf("channel_version=%q want %q", gotPayload.BaseInfo.ChannelVersion, wechatIlinkChannelVersion)
	}
	if gotAuth != "Bearer bot-token" {
		t.Fatalf("authorization=%q", gotAuth)
	}
	if gotAuthType != "ilink_bot_token" {
		t.Fatalf("authorizationType=%q", gotAuthType)
	}
	if gotAppID != wechatIlinkAppID {
		t.Fatalf("appID=%q want %q", gotAppID, wechatIlinkAppID)
	}
	if gotClientVersion != "131329" {
		t.Fatalf("clientVersion=%q", gotClientVersion)
	}
	if strings.TrimSpace(gotUIN) == "" {
		t.Fatalf("expected X-WECHAT-UIN header")
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs)=%d want 1", len(msgs))
	}
	data, ok := msgs[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", msgs[0].Data)
	}
	if data["text"] != "hello from ilink" {
		t.Fatalf("text=%#v", data["text"])
	}
	if data["context_token"] != "context-1" {
		t.Fatalf("context_token=%#v", data["context_token"])
	}
	if data["session_id"] != "session-1" {
		t.Fatalf("session_id=%#v", data["session_id"])
	}
}

func TestWeChatPollingModeLoadsStateAndDispatchesToHandler(t *testing.T) {
	bot, err := NewWeChatBot("wecom")
	if err != nil {
		t.Fatalf("NewWeChatBot: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "wechat.state.json")
	if err := os.WriteFile(path, []byte("{\"next_cursor\":\"cursor-1\"}\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	handler := &fakeWeChatHandler{reply: "ok"}
	bot.SetHandler(handler)
	bot.ConfigurePolling("https://ilinkai.weixin.qq.com/", "bot-token", path, 1)
	bot.ConfigureMode("polling")

	calls := 0
	bot.pollingFetchFn = func(ctx context.Context, cursor string) ([]*protocol.Message, string, error) {
		calls++
		if cursor != "cursor-1" {
			t.Fatalf("cursor=%q want cursor-1", cursor)
		}
		return []*protocol.Message{{
			Kind:   protocol.MessageKindChannel,
			Action: protocol.ActionCreate,
			Data: map[string]interface{}{
				"channel": "wechat",
				"text":    "hello",
			},
		}}, "cursor-2", nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bot.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = bot.Stop() }()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if handler.last != nil && bot.getPollingCursor() == "cursor-2" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if calls == 0 {
		t.Fatalf("expected polling fetch to be called")
	}
	if handler.last == nil {
		t.Fatalf("expected handler to receive polling message")
	}
	if bot.callbackServer != nil {
		t.Fatalf("expected polling mode to skip callback server")
	}
	if got := bot.getPollingCursor(); got != "cursor-2" {
		t.Fatalf("polling cursor=%q want cursor-2", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "cursor-2") {
		t.Fatalf("expected persisted cursor in file: %s", string(data))
	}
}

func TestWeChatPollingModeRequiresBaseURLAndToken(t *testing.T) {
	t.Run("missing baseURL", func(t *testing.T) {
		bot, err := NewWeChatBot("wecom")
		if err != nil {
			t.Fatalf("NewWeChatBot: %v", err)
		}
		bot.ConfigurePolling("", "bot-token", "", 1)
		bot.ConfigureMode("polling")

		err = bot.Start(context.Background())
		if err == nil || !strings.Contains(err.Error(), "baseURL is required") {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		bot, err := NewWeChatBot("wecom")
		if err != nil {
			t.Fatalf("NewWeChatBot: %v", err)
		}
		bot.ConfigurePolling("https://ilinkai.weixin.qq.com/", "", "", 1)
		bot.ConfigureMode("polling")

		err = bot.Start(context.Background())
		if err == nil || !strings.Contains(err.Error(), "token is required") {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}
