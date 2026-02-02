package memory

import (
	"context"
	"fmt"
	"strings"
)

// Searcher executes semantic searches using an embedder and store.
type Searcher struct {
	Embedder Embedder
	Store    *Store
	TopK     int
}

// Search embeds the query and returns top-K results.
func (s *Searcher) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if s == nil || s.Embedder == nil || s.Store == nil {
		return nil, fmt.Errorf("searcher is not configured")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	embeddings, err := s.Embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) != 1 {
		return nil, fmt.Errorf("query embedding missing")
	}
	topK := s.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}
	return s.Store.Search(ctx, embeddings[0], topK)
}
