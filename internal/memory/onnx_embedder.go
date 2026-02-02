package memory

import (
	"errors"
)

// OnnxConfig contains settings for the ONNX embedder.
type OnnxConfig struct {
	ModelPath     string
	TokenizerPath string
	MaxTokens     int
	LibraryPath   string
}

// ErrOnnxUnavailable indicates the ONNX embedder is not yet configured.
var ErrOnnxUnavailable = errors.New("onnx embedder is not configured")

// NewOnnxEmbedder constructs an ONNX runtime embedder.
func NewOnnxEmbedder(cfg OnnxConfig) (Embedder, error) {
	_ = cfg
	return nil, ErrOnnxUnavailable
}
