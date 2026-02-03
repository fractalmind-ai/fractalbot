package memory

import (
	"fmt"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

// HFTokenizer wraps a HuggingFace-compatible tokenizer.json.
type HFTokenizer struct {
	inner *tokenizer.Tokenizer
}

// NewHFTokenizer loads a tokenizer.json file using the pure-Go tokenizer.
func NewHFTokenizer(path string) (*HFTokenizer, error) {
	if path == "" {
		return nil, fmt.Errorf("tokenizer path is required")
	}
	tk, err := pretrained.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer: %w", err)
	}
	return &HFTokenizer{inner: tk}, nil
}

// Encode returns token IDs with special tokens enabled.
func (t *HFTokenizer) Encode(text string) ([]int64, error) {
	if t == nil || t.inner == nil {
		return nil, fmt.Errorf("tokenizer is not initialized")
	}
	encoding, err := t.inner.EncodeSingle(text, true)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(encoding.Ids))
	for i, id := range encoding.Ids {
		ids[i] = int64(id)
	}
	return ids, nil
}
