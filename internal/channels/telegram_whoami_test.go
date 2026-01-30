package channels

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type sendMessagePayload struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

func captureHTTPClient(t *testing.T, payload *sendMessagePayload) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if err := json.Unmarshal(body, payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
}

func TestTelegramWhoAmI(t *testing.T) {
	cases := []struct {
		name      string
		userID    int64
		username  string
		chatID    int64
		adminID   int64
		wantUser  string
		wantName  string
		wantChat  string
		wantAdmin string
	}{
		{
			name:      "admin-with-username",
			userID:    42,
			username:  "alice",
			chatID:    99,
			adminID:   42,
			wantUser:  "User ID: 42",
			wantName:  "Username: @alice",
			wantChat:  "Chat ID: 99",
			wantAdmin: "Is admin: true",
		},
		{
			name:      "non-admin-no-username",
			userID:    7,
			username:  "",
			chatID:    100,
			adminID:   42,
			wantUser:  "User ID: 7",
			wantName:  "Username: (none)",
			wantChat:  "Chat ID: 100",
			wantAdmin: "Is admin: false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bot, err := NewTelegramBot("token", nil, tc.adminID, "qa-1", []string{"qa-1"})
			if err != nil {
				t.Fatalf("NewTelegramBot: %v", err)
			}
			var payload sendMessagePayload
			bot.httpClient = captureHTTPClient(t, &payload)

			msg := &TelegramMessage{
				Text: "/whoami",
				From: &TelegramUser{ID: tc.userID, UserName: tc.username},
				Chat: &TelegramChat{ID: tc.chatID},
			}

			handled, err := bot.handleCommand(msg)
			if !handled {
				t.Fatalf("expected handled")
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if payload.ChatID != tc.chatID {
				t.Fatalf("chat_id=%d want %d", payload.ChatID, tc.chatID)
			}
			if !strings.Contains(payload.Text, tc.wantUser) {
				t.Fatalf("missing user id in reply: %q", payload.Text)
			}
			if !strings.Contains(payload.Text, tc.wantName) {
				t.Fatalf("missing username in reply: %q", payload.Text)
			}
			if !strings.Contains(payload.Text, tc.wantChat) {
				t.Fatalf("missing chat id in reply: %q", payload.Text)
			}
			if !strings.Contains(payload.Text, tc.wantAdmin) {
				t.Fatalf("missing admin flag in reply: %q", payload.Text)
			}
		})
	}
}
