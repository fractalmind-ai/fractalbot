package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	_ = ctx
	embeddings := make([][]float32, 0, len(texts))
	for _, text := range texts {
		embeddings = append(embeddings, []float32{float32(len(text)), 1})
	}
	return embeddings, nil
}

func TestIndexerIndex(t *testing.T) {
	root := t.TempDir()
	memoryRoot := filepath.Join(root, "memory")
	if err := writeFile(filepath.Join(root, "MEMORY.md"), "hello world\nfoo"); err != nil {
		t.Fatalf("write memory.md: %v", err)
	}
	if err := writeFile(filepath.Join(memoryRoot, "notes.md"), "bar baz"); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}

	store, err := OpenStore(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	chunker := Chunker{
		MaxTokens:     4,
		OverlapTokens: 1,
		Counter: TokenCounterFunc(func(text string) (int, error) {
			return len(strings.Fields(text)), nil
		}),
	}

	indexer := Indexer{
		Embedder:  fakeEmbedder{},
		Chunker:   chunker,
		BatchSize: 2,
	}

	count, err := indexer.Index(context.Background(), root, store)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected chunks indexed")
	}
	results, err := store.Search(context.Background(), []float32{1, 1}, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected search results")
	}
}

func writeFile(path, content string) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
