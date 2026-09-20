package policy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type PathMode string

const (
	PathRead         PathMode = "read"
	PathWrite        PathMode = "write"
	PathExternalRead PathMode = "external-read"
)

func assertWorkspacePath(root, declaredPath string, mode PathMode) (string, string, error) {
	if root == "" {
		return "", "", fmt.Errorf("root directory is required")
	}
	if mode != PathRead && mode != PathWrite && mode != PathExternalRead {
		return "", "", fmt.Errorf("unsupported path mode")
	}
	if !validConcretePath(declaredPath) {
		return "", "", fmt.Errorf("path must be a local slash-separated relative path")
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve root: %w", err)
	}
	rootHandle, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return "", "", fmt.Errorf("open root: %w", err)
	}
	defer rootHandle.Close()

	parts := strings.Split(declaredPath, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		info, statErr := rootHandle.Lstat(prefix)
		if statErr != nil {
			if errors.Is(statErr, fs.ErrNotExist) {
				break
			}
			return "", "", fmt.Errorf("path is not contained by root: %w", statErr)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			if _, err := rootHandle.Open(prefix); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", "", fmt.Errorf("symlink is not contained by root: %w", err)
			}
		} else if i < len(parts)-1 && !info.IsDir() {
			return "", "", fmt.Errorf("path component is not a directory")
		}
	}

	local, err := filepath.Localize(declaredPath)
	if err != nil {
		return "", "", fmt.Errorf("localize path: %w", err)
	}
	absolutePath := filepath.Join(absoluteRoot, local)
	matchedRule := "workspace." + string(mode)
	if mode == PathExternalRead {
		matchedRule = "externalRepository.readOnly"
	}
	return absolutePath, matchedRule, nil
}

// AssertWorkspacePath validates a path and checks existing components beneath
// an os.Root. The returned absolute path is diagnostic-only; callers must use
// root-relative os.Root operations for subsequent I/O to avoid TOCTOU races.
func AssertWorkspacePath(root, declaredPath string, mode PathMode) (string, string, error) {
	return assertWorkspacePath(root, declaredPath, mode)
}

func validConcretePath(value string) bool {
	if value == "" || value == "." || strings.Contains(value, "\\") || path.IsAbs(value) || !fs.ValidPath(value) {
		return false
	}
	if len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' {
		return false
	}
	local, err := filepath.Localize(value)
	return err == nil && filepath.IsLocal(local) && !filepath.IsAbs(local) && filepath.VolumeName(local) == ""
}

func validPathPattern(value string) bool {
	if value == "" || value == "." || strings.Contains(value, "\\") || path.IsAbs(value) || !fs.ValidPath(value) {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, "probe"); err != nil {
			return false
		}
	}
	return true
}

func matchesAnyPath(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if matchPathPattern(pattern, value) {
			return true
		}
	}
	return false
}

func matchPathPattern(pattern, value string) bool {
	if !validPathPattern(pattern) || !validConcretePath(value) {
		return false
	}
	return matchPathSegments(strings.Split(pattern, "/"), strings.Split(value, "/"))
}

func matchPathSegments(pattern, value []string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		if matchPathSegments(pattern[1:], value) {
			return true
		}
		return len(value) > 0 && matchPathSegments(pattern, value[1:])
	}
	if len(value) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], value[0])
	return err == nil && matched && matchPathSegments(pattern[1:], value[1:])
}
