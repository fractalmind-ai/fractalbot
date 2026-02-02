package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStoreInsertAndSearch(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	chunks := []IndexedChunk{
		{
			Path:      "MEMORY.md",
			StartLine: 1,
			EndLine:   1,
			Content:   "alpha",
			Embedding: []float32{1, 0},
		},
		{
			Path:      "memory/foo.md",
			StartLine: 1,
			EndLine:   2,
			Content:   "beta",
			Embedding: []float32{0, 1},
		},
	}

	if err := store.InsertChunks(ctx, chunks); err != nil {
		t.Fatalf("insert chunks: %v", err)
	}

	results, err := store.Search(ctx, []float32{1, 0}, 1)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Path != "MEMORY.md" {
		t.Fatalf("unexpected top result path: %s", results[0].Path)
	}
}
