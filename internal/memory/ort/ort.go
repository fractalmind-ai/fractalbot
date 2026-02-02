package ort

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	cacheDirName = "onnxruntime"
)

var (
	extractOnce sync.Once
	extractPath string
	extractErr  error
)

// EnsureLibrary extracts the embedded ORT library into cacheDir and returns its path.
func EnsureLibrary(cacheDir string) (string, error) {
	extractOnce.Do(func() {
		name, data, expectedSHA, err := embeddedLibrary()
		if err != nil {
			extractErr = err
			return
		}
		if cacheDir == "" {
			extractErr = fmt.Errorf("cache dir is required")
			return
		}
		dir := filepath.Join(cacheDir, cacheDirName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			extractErr = fmt.Errorf("failed to create ort cache dir: %w", err)
			return
		}
		path := filepath.Join(dir, name)
		if ok, err := fileMatchesSHA256(path, expectedSHA); err != nil {
			extractErr = err
			return
		} else if !ok {
			if err := os.WriteFile(path, data, 0755); err != nil {
				extractErr = fmt.Errorf("failed to write ort library: %w", err)
				return
			}
			if ok, err := fileMatchesSHA256(path, expectedSHA); err != nil || !ok {
				if err == nil {
					err = fmt.Errorf("checksum mismatch")
				}
				extractErr = fmt.Errorf("failed to verify ort library: %w", err)
				return
			}
		}
		extractPath = path
	})
	if extractErr != nil {
		return "", extractErr
	}
	if extractPath == "" {
		return "", fmt.Errorf("ort library not available")
	}
	return extractPath, nil
}

func fileMatchesSHA256(path, expected string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("%s is a directory", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	return actual == expected, nil
}
