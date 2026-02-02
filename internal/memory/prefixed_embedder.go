package memory

import (
	"context"
	"strings"
)

// PrefixedEmbedder prepends a prefix to each input text.
type PrefixedEmbedder struct {
	Prefix string
	Base   Embedder
}

// Embed applies a prefix and delegates to the base embedder.
func (p PrefixedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if p.Base == nil {
		return nil, ErrOnnxUnavailable
	}
	prefix := strings.TrimSpace(p.Prefix)
	processed := make([]string, 0, len(texts))
	for _, text := range texts {
		if prefix == "" {
			processed = append(processed, text)
			continue
		}
		processed = append(processed, prefix+" "+strings.TrimSpace(text))
	}
	return p.Base.Embed(ctx, processed)
}
