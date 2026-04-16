package channels

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fractalmind-ai/fractalbot/pkg/protocol"
)

func TestIsLongBodyShort(t *testing.T) {
	if IsLongBody("hello") {
		t.Fatal("expected short body")
	}
}

func TestIsLongBodyByChars(t *testing.T) {
	long := strings.Repeat("x", bodyCharThreshold)
	if !IsLongBody(long) {
		t.Fatalf("expected long body at %d chars", bodyCharThreshold)
	}
}

func TestIsLongBodyByLines(t *testing.T) {
	lines := strings.Repeat("line\n", bodyLineThreshold)
	if !IsLongBody(lines) {
		t.Fatalf("expected long body at %d lines", bodyLineThreshold)
	}
}

func TestIsLongBodyBelowThreshold(t *testing.T) {
	text := strings.Repeat("line\n", bodyLineThreshold-2)
	if IsLongBody(text) {
		t.Fatal("expected short body below line threshold")
	}
}

func TestWrapMessageBodyNilMessage(t *testing.T) {
	if err := WrapMessageBody(nil, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrapMessageBodyInline(t *testing.T) {
	msg := &protocol.Message{
		Kind: protocol.MessageKindChannel,
		Data: map[string]interface{}{
			"channel": "slack",
			"text":    "short message",
		},
	}

	if err := WrapMessageBody(msg, ""); err != nil {
		t.Fatalf("WrapMessageBody: %v", err)
	}

	data := msg.Data.(map[string]interface{})
	if data["body_mode"] != BodyModeInline {
		t.Fatalf("body_mode=%v, want %s", data["body_mode"], BodyModeInline)
	}
	if data["body_text"] != "short message" {
		t.Fatalf("body_text=%v", data["body_text"])
	}
	// text should still be present for backward compat.
	if data["text"] != "short message" {
		t.Fatalf("text should be preserved, got %v", data["text"])
	}
}

func TestWrapMessageBodyFilePointer(t *testing.T) {
	bodyDir := t.TempDir()
	longText := strings.Repeat("This is a long line of text.\n", bodyLineThreshold+5)

	msg := &protocol.Message{
		Kind: protocol.MessageKindChannel,
		Data: map[string]interface{}{
			"channel": "telegram",
			"text":    longText,
		},
	}

	if err := WrapMessageBody(msg, bodyDir); err != nil {
		t.Fatalf("WrapMessageBody: %v", err)
	}

	data := msg.Data.(map[string]interface{})
	if data["body_mode"] != BodyModeFilePointer {
		t.Fatalf("body_mode=%v, want %s", data["body_mode"], BodyModeFilePointer)
	}

	bodyFile, ok := data["body_file"].(string)
	if !ok || bodyFile == "" {
		t.Fatal("body_file should be set")
	}
	if !strings.HasPrefix(bodyFile, bodyDir) {
		t.Fatalf("body_file=%s not in bodyDir=%s", bodyFile, bodyDir)
	}

	sha, ok := data["body_sha256"].(string)
	if !ok || sha == "" {
		t.Fatal("body_sha256 should be set")
	}

	// Verify file contents match original text.
	content, err := os.ReadFile(bodyFile)
	if err != nil {
		t.Fatalf("read body file: %v", err)
	}
	if string(content) != longText {
		t.Fatalf("file content mismatch")
	}

	// text should still be present for backward compat.
	if data["text"] != longText {
		t.Fatalf("text should be preserved")
	}
}

func TestWrapMessageBodyEmptyText(t *testing.T) {
	msg := &protocol.Message{
		Kind: protocol.MessageKindChannel,
		Data: map[string]interface{}{
			"channel": "slack",
			"text":    "",
		},
	}

	if err := WrapMessageBody(msg, ""); err != nil {
		t.Fatalf("WrapMessageBody: %v", err)
	}

	data := msg.Data.(map[string]interface{})
	if _, ok := data["body_mode"]; ok {
		t.Fatal("body_mode should not be set for empty text")
	}
}

func TestWrapMessageBodyFallbackOnBadDir(t *testing.T) {
	// Use a directory that cannot be created (nested under a file).
	tmpFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(tmpFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(tmpFile, "subdir")

	longText := strings.Repeat("x", bodyCharThreshold+100)
	msg := &protocol.Message{
		Kind: protocol.MessageKindChannel,
		Data: map[string]interface{}{
			"text": longText,
		},
	}

	if err := WrapMessageBody(msg, badDir); err != nil {
		t.Fatalf("WrapMessageBody: %v", err)
	}

	data := msg.Data.(map[string]interface{})
	// Should fall back to inline.
	if data["body_mode"] != BodyModeInline {
		t.Fatalf("body_mode=%v, want inline fallback", data["body_mode"])
	}
	if data["body_text"] != longText {
		t.Fatal("body_text should contain original text on fallback")
	}
}
