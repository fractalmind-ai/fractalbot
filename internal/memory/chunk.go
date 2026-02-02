package memory

import (
	"fmt"
	"strings"
)

// TokenCounter returns the approximate token count for a text.
type TokenCounter interface {
	CountTokens(text string) (int, error)
}

// TokenCounterFunc adapts a function into a TokenCounter.
type TokenCounterFunc func(text string) (int, error)

func (fn TokenCounterFunc) CountTokens(text string) (int, error) {
	return fn(text)
}

// Chunk represents a chunked piece of text.
type Chunk struct {
	Text       string
	TokenCount int
	StartLine  int
	EndLine    int
}

// Chunker splits text into overlapping token chunks.
type Chunker struct {
	MaxTokens     int
	OverlapTokens int
	Counter       TokenCounter
}

// ChunkText splits the input into chunks using line-based boundaries.
func (c Chunker) ChunkText(text string) ([]Chunk, error) {
	if c.Counter == nil {
		return nil, fmt.Errorf("token counter is required")
	}
	if c.MaxTokens <= 0 {
		return nil, fmt.Errorf("max tokens must be positive")
	}
	overlap := c.OverlapTokens
	if overlap < 0 {
		overlap = 0
	}

	lines := strings.Split(text, "\n")
	var chunks []Chunk
	type lineInfo struct {
		Text    string
		LineNum int
	}
	var current []lineInfo
	currentTokens := 0

	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		linesText := make([]string, 0, len(current))
		for _, entry := range current {
			linesText = append(linesText, entry.Text)
		}
		joined := strings.Join(linesText, "\n")
		count, err := c.Counter.CountTokens(joined)
		if err != nil {
			return err
		}
		chunks = append(chunks, Chunk{
			Text:       joined,
			TokenCount: count,
			StartLine:  current[0].LineNum,
			EndLine:    current[len(current)-1].LineNum,
		})
		return nil
	}

	for i, line := range lines {
		lineTokens, err := c.Counter.CountTokens(line)
		if err != nil {
			return nil, err
		}
		lineNum := i + 1
		if lineTokens > c.MaxTokens && len(current) == 0 {
			chunks = append(chunks, Chunk{
				Text:       line,
				TokenCount: lineTokens,
				StartLine:  lineNum,
				EndLine:    lineNum,
			})
			continue
		}
		if currentTokens+lineTokens > c.MaxTokens && len(current) > 0 {
			if err := flush(); err != nil {
				return nil, err
			}
			if overlap > 0 {
				var overlapLines []lineInfo
				var overlapTokensCount int
				for j := len(current) - 1; j >= 0; j-- {
					tokens, err := c.Counter.CountTokens(current[j].Text)
					if err != nil {
						return nil, err
					}
					if overlapTokensCount+tokens > overlap {
						break
					}
					overlapTokensCount += tokens
					overlapLines = append([]lineInfo{current[j]}, overlapLines...)
				}
				current = overlapLines
				currentTokens = overlapTokensCount
			} else {
				current = nil
				currentTokens = 0
			}
		}
		current = append(current, lineInfo{Text: line, LineNum: lineNum})
		currentTokens += lineTokens
		if i == len(lines)-1 {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}

	return chunks, nil
}
