package memory

import "context"

// Embedder produces embeddings for input texts.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
