package channels

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTelegramPollingOffset_MissingFile(t *testing.T) {
	t.Parallel()

	bot, err := NewTelegramBot("token", nil, 0)
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	dir := t.TempDir()
	bot.pollingOffsetFile = filepath.Join(dir, "telegram.offset")

	if err := bot.loadPollingOffset(); err != nil {
		t.Fatalf("loadPollingOffset: %v", err)
	}
	if bot.nextUpdateID != 0 {
		t.Fatalf("expected nextUpdateID=0, got %d", bot.nextUpdateID)
	}
}

func TestTelegramPollingOffset_LoadsValue(t *testing.T) {
	t.Parallel()

	bot, err := NewTelegramBot("token", nil, 0)
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.offset")
	if err := os.WriteFile(path, []byte("42\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bot.pollingOffsetFile = path

	if err := bot.loadPollingOffset(); err != nil {
		t.Fatalf("loadPollingOffset: %v", err)
	}
	if bot.nextUpdateID != 42 {
		t.Fatalf("expected nextUpdateID=42, got %d", bot.nextUpdateID)
	}
}

func TestTelegramPollingOffset_PersistsValue(t *testing.T) {
	t.Parallel()

	bot, err := NewTelegramBot("token", nil, 0)
	if err != nil {
		t.Fatalf("NewTelegramBot: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "state", "telegram.offset")
	bot.pollingOffsetFile = path
	bot.nextUpdateID = 99

	if err := bot.persistPollingOffset(); err != nil {
		t.Fatalf("persistPollingOffset: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)
	if got != "99\n" {
		t.Fatalf("expected file content %q, got %q", "99\n", got)
	}
}
