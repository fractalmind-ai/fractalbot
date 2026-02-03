//go:build !cgo

package memory

import "fmt"

// NewOnnxEmbedder returns ErrOnnxUnavailable for builds without CGO enabled.
func NewOnnxEmbedder(cfg OnnxConfig) (Embedder, error) {
	_ = cfg
	return nil, fmt.Errorf("%w: build requires CGO_ENABLED=1", ErrOnnxUnavailable)
}

