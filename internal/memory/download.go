package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModelAssets holds resolved local paths for model artifacts.
type ModelAssets struct {
	Dir           string
	ModelPath     string
	TokenizerPath string
}

// EnsureModelAssets downloads and verifies model assets into the cache dir.
func EnsureModelAssets(ctx context.Context, cacheDir string, spec ModelSpec) (ModelAssets, error) {
	if strings.TrimSpace(spec.ID) == "" {
		return ModelAssets{}, fmt.Errorf("model ID is required")
	}
	if strings.TrimSpace(spec.ModelURL) == "" || strings.TrimSpace(spec.ModelSHA256) == "" {
		return ModelAssets{}, fmt.Errorf("model URL and SHA256 are required")
	}
	if strings.TrimSpace(spec.TokenizerURL) == "" || strings.TrimSpace(spec.TokenizerSHA256) == "" {
		return ModelAssets{}, fmt.Errorf("tokenizer URL and SHA256 are required")
	}

	dir, err := ModelDir(cacheDir, spec.ID)
	if err != nil {
		return ModelAssets{}, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ModelAssets{}, fmt.Errorf("failed to create model dir: %w", err)
	}

	modelPath := filepath.Join(dir, DefaultModelFileName)
	tokenizerPath := filepath.Join(dir, DefaultTokenizerFileName)

	client := &http.Client{Timeout: 5 * time.Minute}
	if err := ensureFileWithSHA(ctx, client, modelPath, spec.ModelURL, spec.ModelSHA256); err != nil {
		return ModelAssets{}, err
	}
	if err := ensureFileWithSHA(ctx, client, tokenizerPath, spec.TokenizerURL, spec.TokenizerSHA256); err != nil {
		return ModelAssets{}, err
	}

	return ModelAssets{
		Dir:           dir,
		ModelPath:     modelPath,
		TokenizerPath: tokenizerPath,
	}, nil
}

func ensureFileWithSHA(ctx context.Context, client *http.Client, path, url, expectedSHA string) error {
	if ok, err := fileMatchesSHA256(path, expectedSHA); err != nil {
		return err
	} else if ok {
		return nil
	}

	tmp := path + ".tmp"
	if err := downloadToFile(ctx, client, url, tmp); err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}

	ok, err := fileMatchesSHA256(tmp, expectedSHA)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if !ok {
		_ = os.Remove(tmp)
		return fmt.Errorf("checksum mismatch for %s", filepath.Base(path))
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to finalize download: %w", err)
	}
	return nil
}

func downloadToFile(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	return nil
}

func fileMatchesSHA256(path, expected string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat %s: %w", path, err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("%s is a directory", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, fmt.Errorf("failed to hash %s: %w", path, err)
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	return strings.EqualFold(sum, strings.TrimSpace(expected)), nil
}
