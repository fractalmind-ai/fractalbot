package channels

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

const (
	wechatProviderWeCom        = "wecom"
	wechatProviderMPService    = "mp_service"
	wechatModeCallback         = "callback"
	wechatModePolling          = "polling"
	defaultWeChatPath          = "/wechat/callback"
	defaultWeChatPollInterval  = 3 * time.Second
	maxWeChatBodyBytes         = 1 << 20
	wechatIlinkAppID           = "bot"
	wechatIlinkChannelVersion  = "2.1.1"
	wechatIlinkClientVersion   = 131329
	wechatPollEndpoint         = "ilink/bot/getupdates"
	wechatMessageItemTypeText  = 1
	wechatMessageItemTypeImage = 2
	wechatMessageItemTypeVoice = 3
	wechatMessageItemTypeFile  = 4
	wechatMessageItemTypeVideo = 5
)

// WeChatBot is a minimal lifecycle scaffold for WeChat official providers.
// It currently brings up an optional callback listener so KR2 can progress
// incrementally before inbound parsing/outbound send are implemented.
type WeChatBot struct {
	provider string

	handler IncomingMessageHandler

	mode string

	httpClient *http.Client

	pollingBaseURL         string
	pollingToken           string
	pollingStateFile       string
	pollingIntervalSeconds int
	pollingFetchFn         func(ctx context.Context, cursor string) ([]*protocol.Message, string, error)
	pollingCursorMu        sync.RWMutex
	pollingCursor          string

	callbackListenAddr     string
	callbackPath           string
	callbackToken          string
	callbackEncodingAESKey string
	callbackServer         *http.Server
	callbackServerErr      error

	runningMu sync.RWMutex
	running   bool

	telemetryMu  sync.RWMutex
	lastActivity time.Time
	lastError    time.Time
	lastPollAt   time.Time
	lastPollMsgs int

	stopMu     sync.Mutex
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
}

type WeChatPollingRuntimeStatus struct {
	CursorPresent   bool
	CursorPreview   string
	StateFileExists bool
	LastPollAt      time.Time
	LastPollMsgs    int
}

type wechatPollingState struct {
	NextCursor string `json:"next_cursor,omitempty"`
}

type wechatPollingBaseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

type wechatPollingGetUpdatesRequest struct {
	GetUpdatesBuf string                `json:"get_updates_buf,omitempty"`
	BaseInfo      wechatPollingBaseInfo `json:"base_info,omitempty"`
}

type wechatPollingResponse struct {
	Ret                  int                    `json:"ret,omitempty"`
	Errcode              int                    `json:"errcode,omitempty"`
	Errmsg               string                 `json:"errmsg,omitempty"`
	Msgs                 []wechatPollingMessage `json:"msgs,omitempty"`
	GetUpdatesBuf        string                 `json:"get_updates_buf,omitempty"`
	LongpollingTimeoutMs int                    `json:"longpolling_timeout_ms,omitempty"`
}

type wechatPollingMessage struct {
	MessageID    int64                      `json:"message_id,omitempty"`
	ClientID     string                     `json:"client_id,omitempty"`
	FromUserID   string                     `json:"from_user_id,omitempty"`
	ToUserID     string                     `json:"to_user_id,omitempty"`
	SessionID    string                     `json:"session_id,omitempty"`
	ContextToken string                     `json:"context_token,omitempty"`
	ItemList     []wechatPollingMessageItem `json:"item_list,omitempty"`
}

type wechatPollingMessageItem struct {
	Type      int                     `json:"type,omitempty"`
	TextItem  *wechatPollingTextItem  `json:"text_item,omitempty"`
	VoiceItem *wechatPollingVoiceItem `json:"voice_item,omitempty"`
	FileItem  *wechatPollingFileItem  `json:"file_item,omitempty"`
	ImageItem *wechatPollingImageItem `json:"image_item,omitempty"`
	VideoItem *wechatPollingVideoItem `json:"video_item,omitempty"`
}

type wechatPollingTextItem struct {
	Text string `json:"text,omitempty"`
}

type wechatPollingVoiceItem struct {
	Text string `json:"text,omitempty"`
}

type wechatPollingFileItem struct {
	FileName string `json:"file_name,omitempty"`
}

type wechatPollingImageItem struct{}

type wechatPollingVideoItem struct{}

type wechatCallbackEnvelope struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   string   `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	MsgID        string   `xml:"MsgId"`
	Event        string   `xml:"Event"`
	EventKey     string   `xml:"EventKey"`
	Encrypt      string   `xml:"Encrypt"`
	AgentID      string   `xml:"AgentID"`
}

type wechatCallbackPayload struct {
	Method            string                  `json:"method"`
	Provider          string                  `json:"provider"`
	Path              string                  `json:"path"`
	RequestType       string                  `json:"request_type,omitempty"`
	Query             map[string]string       `json:"query,omitempty"`
	SignatureParam    string                  `json:"signature_param,omitempty"`
	SignaturePresent  bool                    `json:"signature_present"`
	SignatureVerified bool                    `json:"signature_verified"`
	EchostrPresent    bool                    `json:"echostr_present"`
	Envelope          *wechatCallbackEnvelope `json:"envelope,omitempty"`
	ProtocolMessage   *protocol.Message       `json:"protocol_message,omitempty"`
	BodyPreview       string                  `json:"body_preview,omitempty"`
	BodyBytes         int                     `json:"body_bytes,omitempty"`
	TokenConfigured   bool                    `json:"token_configured"`
	AESConfigured     bool                    `json:"aes_key_configured"`
}

func NewWeChatBot(provider string) (*WeChatBot, error) {
	resolved := strings.ToLower(strings.TrimSpace(provider))
	if resolved == "" {
		resolved = wechatProviderWeCom
	}
	switch resolved {
	case wechatProviderWeCom, wechatProviderMPService:
	default:
		return nil, fmt.Errorf("invalid wechat provider: %q", provider)
	}

	bot := &WeChatBot{
		provider:     resolved,
		mode:         wechatModeCallback,
		callbackPath: defaultWeChatPath,
		httpClient:   &http.Client{Timeout: 40 * time.Second},
	}
	bot.pollingFetchFn = bot.fetchPollingUpdates
	return bot, nil
}

func (b *WeChatBot) Name() string { return "wechat" }

func (b *WeChatBot) SetHandler(handler IncomingMessageHandler) {
	b.handler = handler
}

func (b *WeChatBot) IsRunning() bool {
	b.runningMu.RLock()
	defer b.runningMu.RUnlock()
	return b.running
}

func (b *WeChatBot) setRunning(running bool) {
	b.runningMu.Lock()
	b.running = running
	b.runningMu.Unlock()
}

func (b *WeChatBot) LastActivity() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastActivity
}

func (b *WeChatBot) LastError() time.Time {
	b.telemetryMu.RLock()
	defer b.telemetryMu.RUnlock()
	return b.lastError
}

func (b *WeChatBot) markActivity() {
	b.telemetryMu.Lock()
	b.lastActivity = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *WeChatBot) markError() {
	b.telemetryMu.Lock()
	b.lastError = time.Now().UTC()
	b.telemetryMu.Unlock()
}

func (b *WeChatBot) markPoll(msgCount int) {
	b.telemetryMu.Lock()
	b.lastPollAt = time.Now().UTC()
	b.lastPollMsgs = msgCount
	b.telemetryMu.Unlock()
}

func (b *WeChatBot) ConfigureCallback(listenAddr, path, token, encodingAESKey string) {
	b.callbackListenAddr = strings.TrimSpace(listenAddr)
	if trimmedPath := strings.TrimSpace(path); trimmedPath != "" {
		b.callbackPath = trimmedPath
	}
	b.callbackToken = strings.TrimSpace(token)
	b.callbackEncodingAESKey = strings.TrimSpace(encodingAESKey)
}

func (b *WeChatBot) ConfigurePolling(baseURL, token, stateFile string, intervalSeconds int) {
	b.pollingBaseURL = strings.TrimSpace(baseURL)
	b.pollingToken = strings.TrimSpace(token)
	b.pollingStateFile = strings.TrimSpace(stateFile)
	b.pollingIntervalSeconds = intervalSeconds
}

func (b *WeChatBot) ConfigureMode(mode string) {
	resolved := strings.ToLower(strings.TrimSpace(mode))
	switch resolved {
	case "", "auto":
		if b.pollingBaseURL != "" || b.pollingToken != "" || b.pollingStateFile != "" || b.pollingIntervalSeconds > 0 {
			b.mode = wechatModePolling
			return
		}
		b.mode = wechatModeCallback
	case wechatModeCallback, wechatModePolling:
		b.mode = resolved
	default:
		b.mode = wechatModeCallback
	}
}

func (b *WeChatBot) getPollingCursor() string {
	b.pollingCursorMu.RLock()
	defer b.pollingCursorMu.RUnlock()
	return b.pollingCursor
}

func (b *WeChatBot) setPollingCursor(cursor string) {
	b.pollingCursorMu.Lock()
	b.pollingCursor = strings.TrimSpace(cursor)
	b.pollingCursorMu.Unlock()
}

func (b *WeChatBot) Start(ctx context.Context) error {
	if b.IsRunning() {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	b.stopMu.Lock()
	b.stopCancel = cancel
	b.stopMu.Unlock()

	if b.mode == wechatModePolling {
		if err := b.validatePollingConfig(); err != nil {
			b.markError()
			return err
		}
		if err := b.loadPollingState(); err != nil {
			b.markError()
			return err
		}
		b.setRunning(true)
		b.wg.Add(1)
		go b.pollLoop(ctx)
		return nil
	}

	if b.callbackListenAddr == "" {
		b.setRunning(true)
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(b.callbackPath, b.handleCallback)
	server := &http.Server{
		Addr:              b.callbackListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	b.callbackServer = server
	b.callbackServerErr = nil
	b.setRunning(true)

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			b.callbackServerErr = err
			b.markError()
			b.setRunning(false)
		}
	}()

	return nil
}

func (b *WeChatBot) validatePollingConfig() error {
	if strings.TrimSpace(b.pollingBaseURL) == "" {
		return errors.New("wechat polling baseURL is required")
	}
	if strings.TrimSpace(b.pollingToken) == "" {
		return errors.New("wechat polling token is required")
	}
	return nil
}

func (b *WeChatBot) Stop() error {
	b.stopMu.Lock()
	cancel := b.stopCancel
	b.stopCancel = nil
	b.stopMu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.wg.Wait()
	if b.callbackServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := b.callbackServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			b.markError()
			return err
		}
	}
	b.setRunning(false)
	return nil
}

func (b *WeChatBot) pollLoop(ctx context.Context) {
	defer b.wg.Done()

	b.pollOnce(ctx)
	ticker := time.NewTicker(b.pollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.pollOnce(ctx)
		}
	}
}

func (b *WeChatBot) pollInterval() time.Duration {
	if b.pollingIntervalSeconds > 0 {
		return time.Duration(b.pollingIntervalSeconds) * time.Second
	}
	return defaultWeChatPollInterval
}

func (b *WeChatBot) pollOnce(ctx context.Context) {
	cursor := b.getPollingCursor()
	msgs, nextCursor, err := b.pollingFetchFn(ctx, cursor)
	if err != nil {
		b.markError()
		return
	}
	b.markPoll(len(msgs))

	if strings.TrimSpace(nextCursor) != "" && nextCursor != cursor {
		b.setPollingCursor(nextCursor)
		if err := b.persistPollingState(); err != nil {
			b.markError()
		}
	}

	if b.handler == nil {
		return
	}

	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		if _, err := b.handler.HandleIncoming(ctx, msg); err != nil {
			b.markError()
			continue
		}
		b.markActivity()
	}
}

func (b *WeChatBot) PollingRuntimeStatus() WeChatPollingRuntimeStatus {
	stateExists := false
	if path := strings.TrimSpace(b.pollingStateFile); path != "" {
		if _, err := os.Stat(path); err == nil {
			stateExists = true
		}
	}

	b.telemetryMu.RLock()
	lastPollAt := b.lastPollAt
	lastPollMsgs := b.lastPollMsgs
	b.telemetryMu.RUnlock()

	cursor := b.getPollingCursor()
	return WeChatPollingRuntimeStatus{
		CursorPresent:   strings.TrimSpace(cursor) != "",
		CursorPreview:   truncateForPreview(cursor, 64),
		StateFileExists: stateExists,
		LastPollAt:      lastPollAt,
		LastPollMsgs:    lastPollMsgs,
	}
}

func (b *WeChatBot) fetchPollingUpdates(ctx context.Context, cursor string) ([]*protocol.Message, string, error) {
	resp, err := b.doPollingGetUpdates(ctx, cursor)
	if err != nil {
		return nil, cursor, err
	}
	if resp.Errcode != 0 || resp.Ret != 0 {
		return nil, cursor, fmt.Errorf(
			"wechat getupdates failed: ret=%d errcode=%d errmsg=%s",
			resp.Ret,
			resp.Errcode,
			strings.TrimSpace(resp.Errmsg),
		)
	}

	nextCursor := strings.TrimSpace(resp.GetUpdatesBuf)
	if nextCursor == "" {
		nextCursor = cursor
	}

	msgs := make([]*protocol.Message, 0, len(resp.Msgs))
	for _, inbound := range resp.Msgs {
		if mapped := b.pollingMessageToProtocolMessage(inbound); mapped != nil {
			msgs = append(msgs, mapped)
		}
	}
	return msgs, nextCursor, nil
}

func (b *WeChatBot) doPollingGetUpdates(ctx context.Context, cursor string) (*wechatPollingResponse, error) {
	endpoint, err := b.pollingEndpointURL(wechatPollEndpoint)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(wechatPollingGetUpdatesRequest{
		GetUpdatesBuf: strings.TrimSpace(cursor),
		BaseInfo: wechatPollingBaseInfo{
			ChannelVersion: wechatIlinkChannelVersion,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal wechat getupdates payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create wechat getupdates request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("iLink-App-Id", wechatIlinkAppID)
	req.Header.Set("iLink-App-ClientVersion", strconv.Itoa(wechatIlinkClientVersion))
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("X-WECHAT-UIN", randomWeChatPollingUIN())
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(b.pollingToken))

	client := b.httpClient
	if client == nil {
		client = &http.Client{Timeout: 40 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call wechat getupdates: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read wechat getupdates response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("wechat getupdates returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed wechatPollingResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse wechat getupdates response: %w", err)
	}
	return &parsed, nil
}

func (b *WeChatBot) pollingEndpointURL(endpoint string) (string, error) {
	base := strings.TrimSpace(b.pollingBaseURL)
	if base == "" {
		return "", errors.New("wechat polling baseURL is required")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse wechat polling baseURL: %w", err)
	}
	u.Path = path.Join(u.Path, endpoint)
	return u.String(), nil
}

func randomWeChatPollingUIN() string {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	value := binary.BigEndian.Uint32(raw[:])
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(value), 10)))
}

func (b *WeChatBot) pollingMessageToProtocolMessage(msg wechatPollingMessage) *protocol.Message {
	fromUserID := strings.TrimSpace(msg.FromUserID)
	if fromUserID == "" {
		return nil
	}

	text, msgType, attachments := flattenWeChatPollingItems(msg.ItemList)
	if text == "" && len(attachments) == 0 {
		return nil
	}

	messageID := strings.TrimSpace(msg.ClientID)
	if messageID == "" && msg.MessageID != 0 {
		messageID = strconv.FormatInt(msg.MessageID, 10)
	}
	if messageID == "" {
		messageID = fmt.Sprintf("wechat-%d", time.Now().UnixNano())
	}

	data := map[string]interface{}{
		"channel":       "wechat",
		"provider":      b.provider,
		"text":          text,
		"agent":         "",
		"chat_id":       fromUserID,
		"user_id":       fromUserID,
		"username":      fromUserID,
		"message_id":    messageID,
		"chatType":      "dm",
		"msg_type":      msgType,
		"from_user_id":  fromUserID,
		"to_user_id":    strings.TrimSpace(msg.ToUserID),
		"session_id":    strings.TrimSpace(msg.SessionID),
		"context_token": strings.TrimSpace(msg.ContextToken),
	}
	if msg.MessageID != 0 {
		data["upstream_message_id"] = msg.MessageID
	}
	if len(attachments) > 0 {
		data["attachments"] = attachments
	}

	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data:   data,
	}
}

func flattenWeChatPollingItems(items []wechatPollingMessageItem) (string, string, []string) {
	parts := make([]string, 0, len(items))
	attachments := make([]string, 0, len(items))
	msgType := "text"

	for _, item := range items {
		switch item.Type {
		case wechatMessageItemTypeText:
			if item.TextItem != nil && strings.TrimSpace(item.TextItem.Text) != "" {
				parts = append(parts, strings.TrimSpace(item.TextItem.Text))
			}
		case wechatMessageItemTypeVoice:
			msgType = "voice"
			if item.VoiceItem != nil && strings.TrimSpace(item.VoiceItem.Text) != "" {
				parts = append(parts, strings.TrimSpace(item.VoiceItem.Text))
			} else {
				parts = append(parts, "[audio]")
			}
		case wechatMessageItemTypeImage:
			msgType = "image"
			parts = append(parts, "[image]")
			attachments = append(attachments, "image")
		case wechatMessageItemTypeFile:
			msgType = "file"
			name := ""
			if item.FileItem != nil {
				name = strings.TrimSpace(item.FileItem.FileName)
			}
			if name != "" {
				parts = append(parts, fmt.Sprintf("[file: %s]", name))
			} else {
				parts = append(parts, "[file]")
			}
			attachments = append(attachments, "file")
		case wechatMessageItemTypeVideo:
			msgType = "video"
			parts = append(parts, "[video]")
			attachments = append(attachments, "video")
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n")), msgType, attachments
}

func (b *WeChatBot) loadPollingState() error {
	if b.pollingStateFile == "" {
		return nil
	}

	data, err := os.ReadFile(b.pollingStateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to read wechat polling state file: %w", err)
	}

	var state wechatPollingState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse wechat polling state file: %w", err)
	}
	b.setPollingCursor(state.NextCursor)
	return nil
}

func (b *WeChatBot) persistPollingState() error {
	if b.pollingStateFile == "" {
		return nil
	}

	dir := filepath.Dir(b.pollingStateFile)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create wechat polling state dir: %w", err)
		}
	}

	tmp, err := os.CreateTemp(dir, ".wechat-polling-state-*")
	if err != nil {
		return fmt.Errorf("failed to create temp wechat polling state file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	payload, err := json.MarshalIndent(wechatPollingState{
		NextCursor: b.getPollingCursor(),
	}, "", "  ")
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to marshal wechat polling state: %w", err)
	}

	if _, err := tmp.Write(append(payload, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temp wechat polling state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp wechat polling state file: %w", err)
	}
	if err := os.Rename(tmp.Name(), b.pollingStateFile); err != nil {
		return fmt.Errorf("failed to rename temp wechat polling state file: %w", err)
	}
	return nil
}

func (b *WeChatBot) SendMessage(ctx context.Context, target string, text string) error {
	_ = ctx
	_ = target
	_ = text
	b.markError()
	return errors.New("wechat sender not configured yet")
}

func (b *WeChatBot) handleCallback(w http.ResponseWriter, r *http.Request) {
	payload, err := b.parseCallbackRequest(r)
	if err != nil {
		b.markError()
		writeWeChatJSON(w, http.StatusBadRequest, map[string]any{
			"status":   "error",
			"channel":  "wechat",
			"provider": b.provider,
			"error":    err.Error(),
		})
		return
	}

	b.markActivity()
	if payload != nil && payload.RequestType == "handshake" {
		writeWeChatText(w, http.StatusOK, payload.Query["echostr"])
		return
	}

	response := map[string]any{
		"status":   "not_implemented",
		"channel":  "wechat",
		"provider": b.provider,
		"callback": payload,
	}
	if payload != nil && payload.ProtocolMessage != nil && b.handler != nil && r.Method == http.MethodPost {
		reply, handlerErr := b.handler.HandleIncoming(r.Context(), payload.ProtocolMessage)
		if handlerErr != nil {
			b.markError()
			response["handler_error"] = handlerErr.Error()
		} else if strings.TrimSpace(reply) != "" {
			response["handler_reply"] = reply
		}
	}
	writeWeChatJSON(w, http.StatusNotImplemented, response)
}

func (b *WeChatBot) parseCallbackRequest(r *http.Request) (*wechatCallbackPayload, error) {
	query := firstQueryValues(r)
	sigParam, sigValue := detectWeChatSignature(query)
	payload := &wechatCallbackPayload{
		Method:           r.Method,
		Provider:         b.provider,
		Path:             r.URL.Path,
		Query:            query,
		SignatureParam:   sigParam,
		SignaturePresent: strings.TrimSpace(sigValue) != "",
		EchostrPresent:   strings.TrimSpace(query["echostr"]) != "",
		TokenConfigured:  b.callbackToken != "",
		AESConfigured:    b.callbackEncodingAESKey != "",
	}

	switch r.Method {
	case http.MethodGet:
		payload.RequestType = "handshake"
		if err := b.validateCallbackQuery(payload); err != nil {
			return nil, err
		}
		payload.SignatureVerified = true
		return payload, nil
	case http.MethodPost:
		payload.RequestType = "message"
		body, err := io.ReadAll(io.LimitReader(r.Body, maxWeChatBodyBytes))
		if err != nil {
			return nil, fmt.Errorf("read callback body: %w", err)
		}
		payload.BodyBytes = len(body)
		payload.BodyPreview = truncateForPreview(string(body), 240)
		trimmed := strings.TrimSpace(string(body))
		if trimmed != "" {
			var env wechatCallbackEnvelope
			if err := xml.Unmarshal(body, &env); err != nil {
				return nil, fmt.Errorf("parse callback xml: %w", err)
			}
			payload.Envelope = &env
			payload.ProtocolMessage = b.envelopeToProtocolMessage(&env)
		}
		if err := b.validateCallbackQuery(payload); err != nil {
			return nil, err
		}
		payload.SignatureVerified = true
		return payload, nil
	default:
		return nil, fmt.Errorf("unsupported callback method: %s", r.Method)
	}
}

func (b *WeChatBot) validateCallbackQuery(payload *wechatCallbackPayload) error {
	if payload == nil {
		return errors.New("callback payload is required")
	}
	query := payload.Query
	if payload.RequestType == "handshake" && !payload.EchostrPresent {
		return errors.New("missing echostr query parameter")
	}
	if !payload.TokenConfigured {
		return nil
	}
	if !payload.SignaturePresent {
		return errors.New("missing signature query parameter")
	}
	timestamp := strings.TrimSpace(query["timestamp"])
	if timestamp == "" {
		return errors.New("missing timestamp query parameter")
	}
	nonce := strings.TrimSpace(query["nonce"])
	if nonce == "" {
		return errors.New("missing nonce query parameter")
	}
	expected, err := b.expectedCallbackSignature(payload, timestamp, nonce)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(query[payload.SignatureParam]), expected) {
		return errors.New("invalid callback signature")
	}
	return nil
}

func (b *WeChatBot) expectedCallbackSignature(payload *wechatCallbackPayload, timestamp, nonce string) (string, error) {
	if payload == nil {
		return "", errors.New("callback payload is required")
	}
	var pieces []string
	switch payload.SignatureParam {
	case "msg_signature":
		messagePart := ""
		switch payload.RequestType {
		case "handshake":
			messagePart = strings.TrimSpace(payload.Query["echostr"])
		case "message":
			if payload.Envelope != nil {
				messagePart = strings.TrimSpace(payload.Envelope.Encrypt)
			}
		}
		if messagePart == "" {
			return "", errors.New("missing encrypted payload for msg_signature validation")
		}
		pieces = []string{b.callbackToken, timestamp, nonce, messagePart}
	case "signature":
		pieces = []string{b.callbackToken, timestamp, nonce}
		if payload.RequestType == "message" && payload.Envelope != nil && strings.TrimSpace(payload.Envelope.Encrypt) != "" {
			return "", errors.New("signature query parameter is incompatible with encrypted payload")
		}
	default:
		return "", errors.New("missing signature query parameter")
	}
	return computeWeChatSignature(pieces...), nil
}

func detectWeChatSignature(query map[string]string) (string, string) {
	if len(query) == 0 {
		return "", ""
	}
	for _, key := range []string{"msg_signature", "signature"} {
		if value, ok := query[key]; ok {
			return key, value
		}
	}
	return "", ""
}

func computeWeChatSignature(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		filtered = append(filtered, strings.TrimSpace(part))
	}
	sort.Strings(filtered)
	hash := sha1.Sum([]byte(strings.Join(filtered, "")))
	return hex.EncodeToString(hash[:])
}

func (b *WeChatBot) envelopeToProtocolMessage(env *wechatCallbackEnvelope) *protocol.Message {
	if env == nil {
		return nil
	}
	data := map[string]interface{}{
		"channel":        "wechat",
		"provider":       b.provider,
		"text":           env.Content,
		"agent":          "",
		"chat_id":        strings.TrimSpace(env.FromUserName),
		"user_id":        strings.TrimSpace(env.FromUserName),
		"username":       strings.TrimSpace(env.FromUserName),
		"message_id":     strings.TrimSpace(env.MsgID),
		"chatType":       "dm",
		"to_user_name":   strings.TrimSpace(env.ToUserName),
		"from_user_name": strings.TrimSpace(env.FromUserName),
		"msg_type":       strings.TrimSpace(env.MsgType),
	}
	if strings.TrimSpace(env.Event) != "" {
		data["event"] = strings.TrimSpace(env.Event)
	}
	if strings.TrimSpace(env.EventKey) != "" {
		data["event_key"] = strings.TrimSpace(env.EventKey)
	}
	if strings.TrimSpace(env.AgentID) != "" {
		data["agent_id"] = strings.TrimSpace(env.AgentID)
	}
	if strings.TrimSpace(env.Encrypt) != "" {
		data["encrypted"] = true
	}
	return &protocol.Message{
		Kind:   protocol.MessageKindChannel,
		Action: protocol.ActionCreate,
		Data:   data,
	}
}

func firstQueryValues(r *http.Request) map[string]string {
	if r == nil || r.URL == nil {
		return nil
	}
	q := r.URL.Query()
	if len(q) == 0 {
		return nil
	}
	out := make(map[string]string, len(q))
	for key, values := range q {
		if len(values) == 0 {
			out[key] = ""
			continue
		}
		out[key] = values[0]
	}
	return out
}

func truncateForPreview(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit] + "…"
}

func writeWeChatJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeWeChatText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, text)
}
