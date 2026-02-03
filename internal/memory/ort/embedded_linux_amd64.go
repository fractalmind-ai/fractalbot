//go:build linux && amd64

package ort

import _ "embed"

const embeddedLibraryName = "libonnxruntime.so"
const embeddedLibrarySHA256 = "ce3752ba35018ee6d8127ff4cba955b68b9c8b8b0fed8798a8f2e5c4c5a35fa5"

//go:embed lib/linux/amd64/libonnxruntime.so
var embeddedLibraryData []byte

func embeddedLibrary() (string, []byte, string, error) {
	return embeddedLibraryName, embeddedLibraryData, embeddedLibrarySHA256, nil
}
