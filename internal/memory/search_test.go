package memory

import (
	"context"
	"path/filepath"
	"testing"
)

type fixedEmbedder struct {
	Vector []float32
}

func (f fixedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	_ = ctx
	results := make([][]float32, 0, len(texts))
	for range texts {
		results = append(results, f.Vector)
	}
	return results, nil
}

func TestSearcherSearch(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.InsertChunks(context.Background(), []IndexedChunk{{
		Path:      "MEMORY.md",
		StartLine: 1,
		EndLine:   1,
		Content:   "alpha",
		Embedding: []float32{1, 0},
	}}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	searcher := Searcher{
		Embedder: fixedEmbedder{Vector: []float32{1, 0}},
		Store:    store,
		TopK:     1,
	}

	results, err := searcher.Search(context.Background(), "query")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}
