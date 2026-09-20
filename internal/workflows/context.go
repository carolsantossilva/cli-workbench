package workflows

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"station/internal/policy"
	"station/internal/workspace"
)

const maxSnapshotScanBytes = 1 << 20
const maxContextInputBytes = 64 << 10

var errContextContentOmitted = errors.New("context file content is not small regular text")

type Snapshot struct {
	Source       string          `json:"source"`
	Entries      []SnapshotEntry `json:"entries"`
	ChangedPaths []string        `json:"changedPaths,omitempty"`
	OmittedPaths []string        `json:"omittedPaths,omitempty"`
}

type SnapshotEntry struct {
	Path       string    `json:"path"`
	Exists     bool      `json:"exists"`
	SHA256     string    `json:"sha256,omitempty"`
	Bytes      int64     `json:"bytes,omitempty"`
	ModifiedAt time.Time `json:"mtime,omitempty"`
}

type ContextInput struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256,omitempty"`
	Bytes     int64  `json:"bytes"`
	LineCount int    `json:"lineCount"`
	Role      string `json:"role"`
	Content   string `json:"-"`
}

type ContextSummary struct {
	Path        string `json:"path"`
	SummaryKind string `json:"summaryKind"`
	Content     string `json:"content"`
}

type ContextArtifact struct {
	Path       string    `json:"path"`
	Exists     bool      `json:"exists"`
	SHA256     string    `json:"sha256,omitempty"`
	Bytes      int64     `json:"bytes,omitempty"`
	ModifiedAt time.Time `json:"mtime,omitempty"`
	Status     string    `json:"status"`
	HashBefore string    `json:"hashBefore,omitempty"`
	HashAfter  string    `json:"hashAfter,omitempty"`
}

type ContextOmission struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ContextPacket struct {
	WorkflowID      string            `json:"workflowId"`
	WorkspaceRoot   string            `json:"workspaceRoot"`
	InstructionPath string            `json:"instructionPath"`
	Instruction     string            `json:"-"`
	Inputs          []ContextInput    `json:"inputs"`
	Summaries       []ContextSummary  `json:"summaries"`
	AllowedSkills   []string          `json:"allowedSkills"`
	AllowedCommands []string          `json:"allowedCommands"`
	ReadPaths       []string          `json:"readPaths"`
	WritePaths      []string          `json:"writePaths"`
	ExpectedOutputs []ContextArtifact `json:"expectedOutputs"`
	NextHandoff     string            `json:"nextHandoff"`
	Omissions       []ContextOmission `json:"omissions,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
}

type Input struct {
	Manifest *workspace.Workspace
}

type WorkflowResult struct {
	WorkflowID        string            `json:"workflowId"`
	RunID             string            `json:"runId"`
	Status            string            `json:"status"`
	Outputs           []ContextArtifact `json:"outputs"`
	OutOfScopeChanges []string          `json:"outOfScopeChanges,omitempty"`
	Warnings          []string          `json:"warnings,omitempty"`
}

func snapshotWorkspace(root string, paths []string) (Snapshot, error) {
	if snapshot, supported, err := snapshotGitWorkspace(root, paths); supported || err != nil {
		return snapshot, err
	}
	return snapshotFilesystem(root, paths)
}

func snapshotFilesystem(root string, paths []string) (Snapshot, error) {
	if root == "" {
		return Snapshot{}, fmt.Errorf("workspace root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	rootHandle, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open workspace root: %w", err)
	}
	defer rootHandle.Close()

	entries := make(map[string]SnapshotEntry)
	omittedPaths := make(map[string]struct{})
	for _, pattern := range paths {
		if !validSnapshotPattern(pattern) {
			return Snapshot{}, fmt.Errorf("snapshot path must be a relative slash-separated path")
		}
		if excludedContextPath(pattern) {
			if isEnvironmentPath(pattern) {
				omittedPaths[pattern] = struct{}{}
			}
			continue
		}
		info, statErr := rootHandle.Lstat(pattern)
		wildcard := hasSnapshotWildcard(pattern)
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return Snapshot{}, fmt.Errorf("inspect snapshot path: %w", statErr)
		}
		if errors.Is(statErr, fs.ErrNotExist) {
			if wildcard {
				if err := walkSnapshotPattern(absoluteRoot, rootHandle, pattern, entries, omittedPaths); err != nil {
					return Snapshot{}, err
				}
			} else {
				entries[pattern] = SnapshotEntry{Path: pattern}
			}
			continue
		}
		if wildcard {
			if err := walkSnapshotPattern(absoluteRoot, rootHandle, pattern, entries, omittedPaths); err != nil {
				return Snapshot{}, err
			}
			continue
		}
		if info.IsDir() {
			if err := walkSnapshotDirectory(absoluteRoot, rootHandle, pattern, pattern, true, entries, omittedPaths); err != nil {
				return Snapshot{}, err
			}
			continue
		}
		entry, included, err := snapshotFile(absoluteRoot, rootHandle, pattern)
		if err != nil {
			return Snapshot{}, err
		}
		if included {
			if entry.SHA256 == "" {
				omittedPaths[pattern] = struct{}{}
			} else {
				entries[pattern] = entry
			}
		}
	}

	result := Snapshot{Source: "filesystem", Entries: make([]SnapshotEntry, 0, len(entries))}
	for _, entry := range entries {
		result.Entries = append(result.Entries, entry)
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Path < result.Entries[j].Path })
	for omitted := range omittedPaths {
		result.OmittedPaths = append(result.OmittedPaths, omitted)
	}
	sort.Strings(result.OmittedPaths)
	return result, nil
}

func walkSnapshotPattern(rootPath string, root *os.Root, pattern string, entries map[string]SnapshotEntry, omittedPaths map[string]struct{}) error {
	var prefix []string
	for _, segment := range strings.Split(pattern, "/") {
		if strings.ContainsAny(segment, "*?[") {
			break
		}
		prefix = append(prefix, segment)
	}
	directory := strings.Join(prefix, "/")
	if directory == "" {
		directory = "."
	} else {
		info, err := root.Lstat(directory)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect snapshot pattern directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			omittedPaths[directory] = struct{}{}
			return nil
		}
		if !info.IsDir() {
			return nil
		}
	}
	return walkSnapshotDirectory(rootPath, root, directory, pattern, false, entries, omittedPaths)
}

func walkSnapshotDirectory(rootPath string, root *os.Root, directory, pattern string, includeAll bool, entries map[string]SnapshotEntry, omittedPaths map[string]struct{}) error {
	dir, err := root.Open(directory)
	if err != nil {
		return fmt.Errorf("open snapshot directory: %w", err)
	}
	children, err := dir.ReadDir(-1)
	closeErr := dir.Close()
	if err != nil {
		return fmt.Errorf("read snapshot directory: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close snapshot directory: %w", closeErr)
	}
	for _, child := range children {
		relative := child.Name()
		if directory != "." {
			relative = path.Join(directory, child.Name())
		}
		if excludedContextPath(relative) {
			if isEnvironmentPath(relative) {
				omittedPaths[relative] = struct{}{}
			}
			continue
		}
		if child.IsDir() {
			if err := walkSnapshotDirectory(rootPath, root, relative, pattern, includeAll, entries, omittedPaths); err != nil {
				return err
			}
			continue
		}
		if !includeAll && !matchesSnapshotPattern(pattern, relative) {
			continue
		}
		entry, included, err := snapshotFile(rootPath, root, relative)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if included {
			if entry.SHA256 == "" {
				omittedPaths[relative] = struct{}{}
			} else {
				entries[relative] = entry
			}
		}
	}
	return nil
}

func snapshotFile(rootPath string, root *os.Root, relative string) (SnapshotEntry, bool, error) {
	if excludedContextPath(relative) {
		return SnapshotEntry{}, false, nil
	}
	symlink, err := hasSymlinkComponent(root, relative)
	if errors.Is(err, fs.ErrNotExist) {
		return SnapshotEntry{}, false, fs.ErrNotExist
	}
	if err != nil {
		return SnapshotEntry{}, false, fmt.Errorf("inspect snapshot path components: %w", err)
	}
	if symlink {
		return SnapshotEntry{Path: relative, Exists: true}, true, nil
	}
	entryInfo, err := root.Lstat(relative)
	if errors.Is(err, fs.ErrNotExist) {
		return SnapshotEntry{}, false, fs.ErrNotExist
	}
	if err != nil {
		return SnapshotEntry{}, false, fmt.Errorf("inspect snapshot file: %w", err)
	}
	if !entryInfo.Mode().IsRegular() {
		return SnapshotEntry{}, false, nil
	}
	if _, _, err := policy.AssertWorkspacePath(rootPath, relative, policy.PathRead); err != nil {
		return SnapshotEntry{}, false, fmt.Errorf("snapshot path is not contained by workspace: %w", err)
	}
	file, err := root.Open(relative)
	if err != nil {
		return SnapshotEntry{}, false, fmt.Errorf("open snapshot file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return SnapshotEntry{}, false, fmt.Errorf("stat snapshot file: %w", err)
	}
	if !os.SameFile(entryInfo, info) || !info.Mode().IsRegular() {
		return SnapshotEntry{}, false, fmt.Errorf("snapshot file changed while opening")
	}

	entry := SnapshotEntry{
		Path:       relative,
		Exists:     true,
		Bytes:      info.Size(),
		ModifiedAt: info.ModTime().UTC(),
	}
	digest, omitted, err := hashSnapshotContent(relative, file, info.Size())
	if err != nil {
		return SnapshotEntry{}, false, fmt.Errorf("inspect snapshot content: %w", err)
	}
	if omitted {
		return entry, true, nil
	}
	entry.SHA256 = digest
	return entry, true, nil
}

func hashSnapshotContent(relative string, reader io.Reader, size int64) (string, bool, error) {
	probeSize := size
	if probeSize > maxSnapshotScanBytes {
		probeSize = maxSnapshotScanBytes
	}
	var sample bytes.Buffer
	if probeSize > 0 {
		if _, err := io.CopyN(&sample, reader, probeSize); err != nil {
			return "", false, fmt.Errorf("read bounded snapshot probe: %w", err)
		}
	}
	if !isTextContent(sample.Bytes()) || policy.ScanData(relative, sample.String(), "source", policy.DataPolicy{}).Decision.Outcome == policy.Deny {
		return "", true, nil
	}
	hasher := sha256.New()
	if _, err := hasher.Write(sample.Bytes()); err != nil {
		return "", false, err
	}
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", false, fmt.Errorf("hash remaining snapshot content: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), false, nil
}

func hasSymlinkComponent(root *os.Root, relative string) (bool, error) {
	parts := strings.Split(relative, "/")
	var prefix string
	for index, part := range parts {
		if prefix == "" {
			prefix = part
		} else {
			prefix = path.Join(prefix, part)
		}
		info, err := root.Lstat(prefix)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
		if index < len(parts)-1 && !info.IsDir() {
			return false, nil
		}
	}
	return false, nil
}

func validSnapshotPattern(value string) bool {
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

func hasSnapshotWildcard(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func excludedContextPath(value string) bool {
	for _, segment := range strings.Split(path.Clean(value), "/") {
		switch segment {
		case ".station", ".git", "node_modules", ".cache", "cache", "__pycache__", ".pytest_cache", ".mypy_cache", ".turbo", ".expo":
			return true
		}
	}
	return strings.HasPrefix(path.Base(value), ".env")
}

func matchesSnapshotPattern(pattern, value string) bool {
	return matchSnapshotSegments(strings.Split(pattern, "/"), strings.Split(value, "/"))
}

func matchSnapshotSegments(pattern, value []string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		if matchSnapshotSegments(pattern[1:], value) {
			return true
		}
		return len(value) > 0 && matchSnapshotSegments(pattern, value[1:])
	}
	if len(value) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], value[0])
	return err == nil && matched && matchSnapshotSegments(pattern[1:], value[1:])
}

func fileLineCount(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	return bytes.Count(content, []byte("\n")) + 1
}

func isTextContent(content []byte) bool {
	if bytes.ContainsRune(content, 0) || !utf8.Valid(content) {
		return false
	}
	for _, value := range string(content) {
		if unicode.IsControl(value) && value != '\n' && value != '\r' && value != '\t' {
			return false
		}
	}
	return true
}
