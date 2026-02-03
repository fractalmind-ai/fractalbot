package memory

import "errors"

const (
	defaultMaxTokens = 512
)

// OnnxConfig contains settings for the ONNX embedder.
type OnnxConfig struct {
	ModelPath     string
	TokenizerPath string
	Tokenizer     Tokenizer
	MaxTokens     int
	LibraryPath   string
	CacheDir      string
}

// ErrOnnxUnavailable indicates ONNX runtime support is not available in this build.
var ErrOnnxUnavailable = errors.New("onnx embedder is not configured")

