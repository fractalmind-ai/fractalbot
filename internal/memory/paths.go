package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultCacheDirName = "fractalbot"
	defaultModelDirName = "embeddings"
	defaultIndexDirName = "memory-index"
)

// ResolveCacheDir returns the cache directory for memory assets.
func ResolveCacheDir(cacheDir string) (string, error) {
	if strings.TrimSpace(cacheDir) != "" {
		return expandUser(cacheDir)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("failed to resolve cache dir: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, defaultCacheDirName), nil
}

// ModelDir returns the model directory for the given model ID.
func ModelDir(cacheDir, modelID string) string {
	return filepath.Join(cacheDir, defaultModelDirName, modelID)
}

// IndexPath returns the index path for a model ID.
func IndexPath(cacheDir, modelID string) string {
	return filepath.Join(cacheDir, defaultIndexDirName, modelID, "index.db")
}

func expandUser(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home dir: %w", err)
	}
	if trimmed == "~" {
		return home, nil
	}
	if strings.HasPrefix(trimmed, "~/") {
		return filepath.Join(home, trimmed[2:]), nil
	}
	return filepath.Join(home, trimmed[1:]), nil
}
