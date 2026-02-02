package memory

import "fmt"

// Tokenizer converts text into token IDs.
type Tokenizer interface {
	Encode(text string) ([]int64, error)
}

// TokenizerCounter adapts a Tokenizer into a TokenCounter.
type TokenizerCounter struct {
	Tokenizer Tokenizer
}

func (t TokenizerCounter) CountTokens(text string) (int, error) {
	if t.Tokenizer == nil {
		return 0, fmt.Errorf("tokenizer is nil")
	}
	ids, err := t.Tokenizer.Encode(text)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}
