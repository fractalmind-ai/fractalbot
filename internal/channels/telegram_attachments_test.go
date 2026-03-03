package channels

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

type captureTelegramInboundHandler struct {
	called bool
	last   *protocol.Message
}

func (h *captureTelegramInboundHandler) HandleIncoming(ctx context.Context, msg *protocol.Message) (string, error) {
	_ = ctx
	h.called = true
	h.last = msg
	return "", nil
}

func TestTelegramDocumentAttachmentIncludedInProtocolMessage(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	bot.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/getFile") {
				t.Fatalf("unexpected request path %q", req.URL.Path)
			}
			if req.URL.Query().Get("file_id") != "doc123" {
				t.Fatalf("file_id=%q", req.URL.Query().Get("file_id"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"ok":true,"result":{"file_id":"doc123","file_path":"documents/test.pdf"}}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
	}

	handler := &captureTelegramInboundHandler{}
	bot.SetHandler(handler)

	bot.handleIncomingMessage(&TelegramMessage{
		Text: "/agent qa-1 process this file",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55, Type: "private"},
		Document: &TelegramDocument{
			FileID:   "doc123",
			FileName: "test.pdf",
			MimeType: "application/pdf",
		},
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if handler.last == nil || len(handler.last.Attachments) != 1 {
		t.Fatalf("attachments=%v", handler.last.Attachments)
	}
	attachment := handler.last.Attachments[0]
	if attachment.Type != "file" {
		t.Fatalf("attachment.type=%q", attachment.Type)
	}
	if attachment.URL != "https://api.telegram.org/file/bottoken/documents/test.pdf" {
		t.Fatalf("attachment.url=%q", attachment.URL)
	}
	if attachment.Filename != "test.pdf" {
		t.Fatalf("attachment.filename=%q", attachment.Filename)
	}
	data, ok := handler.last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map data, got %T", handler.last.Data)
	}
	dataAttachments, ok := data["attachments"].([]protocol.Attachment)
	if !ok || len(dataAttachments) != 1 {
		t.Fatalf("data attachments=%v", data["attachments"])
	}
}

func TestTelegramPhotoAttachmentUsesLargestPhoto(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	requestedFileIDs := make([]string, 0, 2)
	bot.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/getFile") {
				t.Fatalf("unexpected request path %q", req.URL.Path)
			}
			fileID := req.URL.Query().Get("file_id")
			requestedFileIDs = append(requestedFileIDs, fileID)
			if fileID != "large-photo-id" {
				t.Fatalf("expected only large photo file_id, got %q", fileID)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"ok":true,"result":{"file_id":"large-photo-id","file_path":"photos/large.jpg"}}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
	}

	handler := &captureTelegramInboundHandler{}
	bot.SetHandler(handler)

	bot.handleIncomingMessage(&TelegramMessage{
		Text: "/agent qa-1 analyze image",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55, Type: "private"},
		Photo: []TelegramPhotoSize{
			{FileID: "small-photo-id", Width: 100, Height: 100, FileSize: 1024},
			{FileID: "large-photo-id", Width: 1200, Height: 1200, FileSize: 4096},
		},
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if len(requestedFileIDs) != 1 {
		t.Fatalf("requestedFileIDs=%v", requestedFileIDs)
	}
	if len(handler.last.Attachments) != 1 {
		t.Fatalf("attachments=%v", handler.last.Attachments)
	}
	if handler.last.Attachments[0].Type != "image" {
		t.Fatalf("attachment.type=%q", handler.last.Attachments[0].Type)
	}
}

func TestTelegramAttachmentResolutionFailureDoesNotBlockMessage(t *testing.T) {
	bot, err := NewTelegramBot("token", []int64{123}, 0, "qa-1", []string{"qa-1"})
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	bot.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/getFile") {
				t.Fatalf("unexpected request path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"description":"boom"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	handler := &captureTelegramInboundHandler{}
	bot.SetHandler(handler)

	bot.handleIncomingMessage(&TelegramMessage{
		Text: "/agent qa-1 continue",
		From: &TelegramUser{ID: 123},
		Chat: &TelegramChat{ID: 55, Type: "private"},
		Document: &TelegramDocument{
			FileID:   "doc123",
			FileName: "test.pdf",
			MimeType: "application/pdf",
		},
	})

	if !handler.called {
		t.Fatalf("expected handler to be called")
	}
	if len(handler.last.Attachments) != 0 {
		t.Fatalf("expected no attachments on getFile failure, got %v", handler.last.Attachments)
	}
}
