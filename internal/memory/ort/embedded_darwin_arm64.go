//go:build darwin && arm64

package ort

import _ "embed"

const embeddedLibraryName = "libonnxruntime.dylib"
const embeddedLibrarySHA256 = "033c67e1b06420827a00cffe9e6c63895b3fd0ef5d2044e392822a35b354d48d"

//go:embed lib/darwin/arm64/libonnxruntime.dylib
var embeddedLibraryData []byte

func embeddedLibrary() (string, []byte, string, error) {
	return embeddedLibraryName, embeddedLibraryData, embeddedLibrarySHA256, nil
}
