//go:build windows && amd64

package ort

import _ "embed"

const embeddedLibraryName = "onnxruntime.dll"
const embeddedLibrarySHA256 = "6135d3c08003afb23b7c8997bed44d15d860e9bf8408aaa87e44fc8e4fe2fa48"

//go:embed lib/windows/amd64/onnxruntime.dll
var embeddedLibraryData []byte

func embeddedLibrary() (string, []byte, string, error) {
	return embeddedLibraryName, embeddedLibraryData, embeddedLibrarySHA256, nil
}
