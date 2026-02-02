package memory

import (
	"strings"
	"testing"
)

func TestChunkerChunkTextWithOverlap(t *testing.T) {
	counter := TokenCounterFunc(func(text string) (int, error) {
		return len(strings.Fields(text)), nil
	})

	chunker := Chunker{
		MaxTokens:     4,
		OverlapTokens: 2,
		Counter:       counter,
	}

	chunks, err := chunker.ChunkText("a b\nc d\ne f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Text != "a b\nc d" {
		t.Fatalf("unexpected chunk 0: %q", chunks[0].Text)
	}
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 2 {
		t.Fatalf("unexpected chunk 0 lines: %d-%d", chunks[0].StartLine, chunks[0].EndLine)
	}
	if chunks[1].Text != "c d\ne f" {
		t.Fatalf("unexpected chunk 1: %q", chunks[1].Text)
	}
	if chunks[1].StartLine != 2 || chunks[1].EndLine != 3 {
		t.Fatalf("unexpected chunk 1 lines: %d-%d", chunks[1].StartLine, chunks[1].EndLine)
	}
}

func TestChunkerRequiresCounter(t *testing.T) {
	chunker := Chunker{MaxTokens: 10}
	if _, err := chunker.ChunkText("hello"); err == nil {
		t.Fatal("expected error for missing counter")
	}
}
