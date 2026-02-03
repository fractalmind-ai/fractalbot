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
func ModelDir(cacheDir, modelID string) (string, error) {
	if err := ValidateModelID(modelID); err != nil {
		return "", err
	}
	target := filepath.Join(cacheDir, defaultModelDirName, modelID)
	if err := ensureUnderRoot(cacheDir, target); err != nil {
		return "", err
	}
	return target, nil
}

// IndexPath returns the index path for a model ID.
func IndexPath(cacheDir, modelID string) (string, error) {
	if err := ValidateModelID(modelID); err != nil {
		return "", err
	}
	target := filepath.Join(cacheDir, defaultIndexDirName, modelID, "index.db")
	if err := ensureUnderRoot(cacheDir, target); err != nil {
		return "", err
	}
	return target, nil
}

// ValidateModelID enforces safe model identifiers for cache paths.
func ValidateModelID(modelID string) error {
	trimmed := strings.TrimSpace(modelID)
	if trimmed == "" {
		return fmt.Errorf("model ID is required")
	}
	if filepath.IsAbs(trimmed) {
		return fmt.Errorf("model ID must be relative")
	}
	if strings.Contains(trimmed, "..") {
		return fmt.Errorf("model ID must not contain '..'")
	}
	if strings.ContainsAny(trimmed, `/\\`) {
		return fmt.Errorf("model ID must not contain path separators")
	}
	for _, ch := range trimmed {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		if ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return fmt.Errorf("model ID contains invalid character %q", ch)
	}
	return nil
}

func ensureUnderRoot(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("failed to resolve cache root: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("failed to resolve cache path: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return fmt.Errorf("failed to resolve cache relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("cache path escapes root")
	}
	return nil
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
