package modes

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"workbench/internal/policy"
	"workbench/internal/workspace"
)

const maxMetadataBytes = 64 << 10
const maxInspectionEntries = 10_000

var bootstrapMetadataFiles = map[string]struct{}{
	"AGENTS.md":           {},
	"README.md":           {},
	"go.mod":              {},
	"go.work":             {},
	"package.json":        {},
	"pnpm-workspace.yaml": {},
	"app.json":            {},
}

type InspectionEntry struct {
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	Bytes        int64  `json:"bytes,omitempty"`
	MetadataRead bool   `json:"metadataRead,omitempty"`
}

type ExistingInspection struct {
	Root           string              `json:"root"`
	ManifestStatus string              `json:"manifestStatus"`
	Projects       []workspace.Project `json:"projects,omitempty"`
	Entries        []InspectionEntry   `json:"entries"`
	Warnings       []string            `json:"warnings,omitempty"`
}

func inspectExistingWorkspace(root string) (ExistingInspection, error) {
	if root == "" {
		return ExistingInspection{}, errors.New("workspace path is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("inspect workspace path: %w", err)
	}
	if !info.IsDir() {
		return ExistingInspection{}, errors.New("workspace path is not a directory")
	}
	rootHandle, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("open workspace root: %w", err)
	}
	defer rootHandle.Close()

	manifestInfo, err := rootHandle.Lstat("workspace.json")
	if errors.Is(err, fs.ErrNotExist) {
		return inspectWithoutManifest(absoluteRoot, rootHandle)
	}
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("inspect workspace.json: %w", err)
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return ExistingInspection{}, errors.New("workspace.json must be a regular, non-symlink file")
	}
	manifestFile, err := rootHandle.Open("workspace.json")
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("open workspace.json within workspace root: %w", err)
	}
	defer manifestFile.Close()
	openedInfo, err := manifestFile.Stat()
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("stat workspace.json: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(manifestInfo, openedInfo) {
		return ExistingInspection{}, errors.New("workspace.json changed while opening")
	}
	manifestBytes, err := io.ReadAll(manifestFile)
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("read workspace.json: %w", err)
	}
	manifest, err := workspace.LoadWorkspaceBytes(manifestBytes)
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("workspace.json is invalid: %w", err)
	}
	diagnostics := workspace.ValidateWorkspace(manifest, absoluteRoot)
	diagnostics = append(diagnostics, policy.ValidateManifestPolicies(manifest)...)
	if len(diagnostics) > 0 {
		return ExistingInspection{}, fmt.Errorf("workspace.json is invalid at %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	paths := declaredReadPaths(manifest)
	entries, err := inspectDeclaredPaths(rootHandle, paths)
	if err != nil {
		return ExistingInspection{}, err
	}
	projects := append([]workspace.Project(nil), manifest.Projects...)
	return ExistingInspection{
		Root: absoluteRoot, ManifestStatus: "valid", Projects: projects, Entries: entries,
		Warnings: []string{"Inspection is read-only and does not execute commands; host tools remain outside Workbench control."},
	}, nil
}

func InspectExistingWorkspace(root string) (ExistingInspection, error) {
	return inspectExistingWorkspace(root)
}

func inspectWithoutManifest(root string, rootHandle *os.Root) (ExistingInspection, error) {
	directory, err := rootHandle.Open(".")
	if err != nil {
		return ExistingInspection{}, fmt.Errorf("list workspace root: %w", err)
	}
	children, readErr := directory.ReadDir(maxInspectionEntries + 1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return ExistingInspection{}, fmt.Errorf("list workspace root: %w", readErr)
	}
	if closeErr != nil {
		return ExistingInspection{}, fmt.Errorf("close workspace root listing: %w", closeErr)
	}
	if len(children) > maxInspectionEntries {
		return ExistingInspection{}, errors.New("inspection exceeded the immediate-root entry limit")
	}
	entries := make([]InspectionEntry, 0, len(children))
	for _, child := range children {
		name := child.Name()
		entry, err := inspectRootEntry(rootHandle, name)
		if err != nil {
			return ExistingInspection{}, err
		}
		if _, allowed := bootstrapMetadataFiles[name]; allowed && entry.Kind == "file" && entry.Bytes <= maxMetadataBytes {
			if err := readMetadataFile(rootHandle, name, entry.Bytes); err != nil {
				return ExistingInspection{}, err
			}
			entry.MetadataRead = true
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return ExistingInspection{
		Root: root, ManifestStatus: "absent", Entries: entries,
		Warnings: []string{"No workspace.json was found; only an immediate, metadata-only inventory and the fixed bootstrap metadata allowlist were inspected."},
	}, nil
}

func inspectDeclaredPaths(root *os.Root, patterns []string) ([]InspectionEntry, error) {
	if len(patterns) > maxInspectionEntries {
		return nil, errors.New("inspection exceeded the declared-path entry limit")
	}
	entries := make(map[string]InspectionEntry)
	visited := 0
	for _, pattern := range patterns {
		if !validInspectionPattern(pattern) {
			return nil, fmt.Errorf("workflow read path is invalid: %q", pattern)
		}
		prefix := literalPrefix(pattern)
		if prefix == "" {
			prefix = "."
		}
		info, err := root.Lstat(filepath.FromSlash(prefix))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect declared path %s: %w", pattern, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			if policy.PathMatches([]string{pattern}, prefix) {
				if err := storeInspectionEntry(entries, inspectionEntry(prefix, info)); err != nil {
					return nil, err
				}
			}
			continue
		}
		if !hasInspectionWildcard(pattern) {
			if policy.PathMatches([]string{pattern}, prefix) {
				if err := storeInspectionEntry(entries, inspectionEntry(prefix, info)); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := walkDeclaredDirectory(root, prefix, pattern, entries, &visited); err != nil {
			return nil, err
		}
	}
	result := make([]InspectionEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func walkDeclaredDirectory(root *os.Root, directory, pattern string, entries map[string]InspectionEntry, visited *int) error {
	handle, err := root.Open(filepath.FromSlash(directory))
	if err != nil {
		return fmt.Errorf("open declared directory %s: %w", directory, err)
	}
	remaining := maxInspectionEntries - *visited
	if remaining < 0 {
		_ = handle.Close()
		return errors.New("inspection exceeded the declared-path entry limit")
	}
	children, readErr := handle.ReadDir(remaining + 1)
	closeErr := handle.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("read declared directory %s: %w", directory, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close declared directory %s: %w", directory, closeErr)
	}
	if len(children) > remaining {
		return errors.New("inspection exceeded the declared-path entry limit")
	}
	*visited += len(children)
	for _, child := range children {
		name := path.Join(directory, child.Name())
		info, err := root.Lstat(filepath.FromSlash(name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect declared entry %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if policy.PathMatches([]string{pattern}, name) {
				if err := storeInspectionEntry(entries, inspectionEntry(name, info)); err != nil {
					return err
				}
			}
			continue
		}
		if info.IsDir() {
			if patternMayMatchDescendant(pattern, name) {
				if err := walkDeclaredDirectory(root, name, pattern, entries, visited); err != nil {
					return err
				}
			}
			continue
		}
		if policy.PathMatches([]string{pattern}, name) {
			if err := storeInspectionEntry(entries, inspectionEntry(name, info)); err != nil {
				return err
			}
		}
	}
	return nil
}

func storeInspectionEntry(entries map[string]InspectionEntry, entry InspectionEntry) error {
	entries[entry.Path] = entry
	if len(entries) > maxInspectionEntries {
		return errors.New("inspection exceeded the declared-path entry limit")
	}
	return nil
}

func patternMayMatchDescendant(pattern, directory string) bool {
	patterns := strings.Split(pattern, "/")
	segments := strings.Split(directory, "/")
	states := map[int]struct{}{0: {}}
	states = inspectionPatternClosure(patterns, states)
	for _, segment := range segments {
		next := make(map[int]struct{})
		for index := range states {
			if index >= len(patterns) {
				continue
			}
			if patterns[index] == "**" {
				next[index] = struct{}{}
				continue
			}
			matched, err := path.Match(patterns[index], segment)
			if err == nil && matched {
				next[index+1] = struct{}{}
			}
		}
		states = inspectionPatternClosure(patterns, next)
		if len(states) == 0 {
			return false
		}
	}
	for index := range states {
		if index < len(patterns) {
			return true
		}
	}
	return false
}

func inspectionPatternClosure(patterns []string, states map[int]struct{}) map[int]struct{} {
	for index := range states {
		for next := index; next < len(patterns) && patterns[next] == "**"; next++ {
			states[next+1] = struct{}{}
		}
	}
	return states
}

func inspectRootEntry(root *os.Root, name string) (InspectionEntry, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return InspectionEntry{}, fmt.Errorf("inspect root entry %s: %w", name, err)
	}
	return inspectionEntry(name, info), nil
}

func inspectionEntry(name string, info os.FileInfo) InspectionEntry {
	kind := "other"
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		kind = "symlink"
	case info.IsDir():
		kind = "directory"
	case info.Mode().IsRegular():
		kind = "file"
	}
	entry := InspectionEntry{Path: filepath.ToSlash(name), Kind: kind}
	if info.Mode().IsRegular() {
		entry.Bytes = info.Size()
	}
	return entry
}

func readMetadataFile(root *os.Root, name string, expectedSize int64) error {
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return fmt.Errorf("read allowlisted metadata %s: %w", name, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat allowlisted metadata %s: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Size() != expectedSize || info.Size() > maxMetadataBytes {
		return fmt.Errorf("allowlisted metadata changed while being inspected: %s", name)
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(file, maxMetadataBytes+1)); err != nil {
		return fmt.Errorf("read allowlisted metadata %s: %w", name, err)
	}
	return nil
}

func declaredReadPaths(manifest *workspace.Workspace) []string {
	paths := make(map[string]struct{})
	for _, id := range sortedWorkflowIDs(manifest.Workflows) {
		for _, value := range manifest.Workflows[id].ReadPaths {
			paths[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(paths))
	for value := range paths {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedWorkflowIDs(workflows map[string]workspace.Workflow) []string {
	ids := make([]string, 0, len(workflows))
	for id := range workflows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func validInspectionPattern(value string) bool {
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

func literalPrefix(pattern string) string {
	var prefix []string
	for _, segment := range strings.Split(pattern, "/") {
		if strings.ContainsAny(segment, "*?[") {
			break
		}
		prefix = append(prefix, segment)
	}
	return strings.Join(prefix, "/")
}

func hasInspectionWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}
