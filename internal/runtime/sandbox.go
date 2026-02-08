package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathSandbox enforces filesystem access under configured roots.
type PathSandbox struct {
	Roots []string
}

// ValidatePath validates candidate paths and returns a safe, absolute path.
func (s PathSandbox) ValidatePath(candidate string) (string, error) {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if len(s.Roots) == 0 {
		return "", fmt.Errorf("sandbox roots are not configured (set agents.runtime.sandboxRoots)")
	}

	var lastErr error
	for _, root := range s.Roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		resolved, err := validateUnderRoot(root, trimmed)
		if err == nil {
			return resolved, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("sandbox roots are not configured (set agents.runtime.sandboxRoots)")
}

func validateUnderRoot(root, candidate string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sandbox root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sandbox root: %w", err)
	}

	absCandidate := candidate
	if !filepath.IsAbs(absCandidate) && !isWindowsAbsPath(absCandidate) {
		absCandidate = filepath.Join(absRoot, absCandidate)
	}
	absCandidate = filepath.Clean(absCandidate)

	candidateReal, err := resolveCandidatePath(absCandidate)
	if err != nil {
		return "", err
	}

	if err := ensureUnderRoot(rootReal, candidateReal); err != nil {
		return "", err
	}
	return candidateReal, nil
}

func resolveCandidatePath(candidate string) (string, error) {
	current := candidate
	var suffix []string
	for {
		info, err := os.Stat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("failed to resolve path: %w", err)
			}
			if info.IsDir() {
				return filepath.Join(append([]string{resolved}, suffix...)...), nil
			}
			if len(suffix) > 0 {
				return "", fmt.Errorf("failed to resolve parent path")
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to access path: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("failed to resolve parent path")
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}

func ensureUnderRoot(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("failed to resolve sandbox relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes sandbox root")
	}
	return nil
}

func isWindowsAbsPath(path string) bool {
	if strings.HasPrefix(path, `\\`) {
		return true
	}
	if len(path) < 3 {
		return false
	}
	drive := path[0]
	if !((drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')) {
		return false
	}
	if path[1] != ':' {
		return false
	}
	return path[2] == '\\' || path[2] == '/'
}
