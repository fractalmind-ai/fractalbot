//go:build !(darwin && arm64) && !(linux && amd64) && !(windows && amd64)

package ort

import "fmt"

func embeddedLibrary() (string, []byte, string, error) {
	return "", nil, "", fmt.Errorf("onnxruntime embedded library not available for this platform")
}
