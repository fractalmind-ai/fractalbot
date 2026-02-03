package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultChunkTokens  = 400
	DefaultChunkOverlap = 80
	DefaultTopK         = 5
	DefaultBatchSize    = 16
)

// Indexer builds a vector index from memory files.
type Indexer struct {
	Embedder  Embedder
	Chunker   Chunker
	BatchSize int
}

// Index scans memory files under root, embeds chunks, and stores them.
func (i *Indexer) Index(ctx context.Context, root string, store *Store) (int, error) {
	if i == nil || i.Embedder == nil {
		return 0, fmt.Errorf("embedder is required")
	}
	if store == nil {
		return 0, fmt.Errorf("store is required")
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	files, err := FindMemoryFiles(root)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}

	if err := store.Reset(ctx); err != nil {
		return 0, err
	}

	batchSize := i.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	totalChunks := 0
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return totalChunks, fmt.Errorf("failed to read %s: %w", path, err)
		}
		chunks, err := i.Chunker.ChunkText(string(content))
		if err != nil {
			return totalChunks, fmt.Errorf("failed to chunk %s: %w", path, err)
		}
		if len(chunks) == 0 {
			continue
		}

		for start := 0; start < len(chunks); start += batchSize {
			end := start + batchSize
			if end > len(chunks) {
				end = len(chunks)
			}
			texts := make([]string, 0, end-start)
			for _, chunk := range chunks[start:end] {
				texts = append(texts, chunk.Text)
			}
			embeddings, err := i.Embedder.Embed(ctx, texts)
			if err != nil {
				return totalChunks, fmt.Errorf("failed to embed %s: %w", path, err)
			}
			if len(embeddings) != len(texts) {
				return totalChunks, fmt.Errorf("embedding count mismatch for %s", path)
			}
			indexed := make([]IndexedChunk, 0, len(texts))
			for idx, embedding := range embeddings {
				chunk := chunks[start+idx]
				indexed = append(indexed, IndexedChunk{
					Path:       path,
					StartLine:  chunk.StartLine,
					EndLine:    chunk.EndLine,
					Content:    chunk.Text,
					TokenCount: chunk.TokenCount,
					Embedding:  embedding,
				})
			}
			if err := store.InsertChunks(ctx, indexed); err != nil {
				return totalChunks, err
			}
			totalChunks += len(indexed)
		}
	}

	return totalChunks, nil
}

// FindMemoryFiles returns MEMORY.md and memory/**/*.md from the root.
func FindMemoryFiles(root string) ([]string, error) {
	var files []string
	memoryFile := filepath.Join(root, "MEMORY.md")
	if info, err := os.Stat(memoryFile); err == nil && !info.IsDir() {
		files = append(files, memoryFile)
	}

	memoryDir := filepath.Join(root, "memory")
	if info, err := os.Stat(memoryDir); err == nil && info.IsDir() {
		err := filepath.WalkDir(memoryDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan memory dir: %w", err)
		}
	}

	sort.Strings(files)
	return files, nil
}
