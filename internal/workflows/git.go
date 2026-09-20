package workflows

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func snapshotGitWorkspace(root string, paths []string) (Snapshot, bool, error) {
	if root == "" {
		return Snapshot{}, false, nil
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return Snapshot{}, false, nil
	}
	version, err := snapshotGitVersion(gitPath)
	if err != nil || !supportsGitWorkspaceSnapshot(version) {
		return Snapshot{}, false, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, false, nil
	}
	topLevel, err := runSnapshotGit(gitPath, absRoot, "rev-parse", "--show-toplevel")
	if err != nil || !snapshotGitTopLevelMatches(absRoot, topLevel) {
		return Snapshot{}, false, nil
	}

	head, err := runSnapshotGit(gitPath, absRoot, "rev-parse", "--verify", "--quiet", "HEAD")
	hasHead := err == nil
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return Snapshot{}, true, fmt.Errorf("check Git HEAD: %w", err)
		}
	} else if len(bytes.TrimSpace(head)) == 0 {
		return Snapshot{}, true, fmt.Errorf("check Git HEAD: Git returned an empty revision")
	}

	status, err := runSnapshotGit(gitPath, absRoot,
		"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return Snapshot{}, true, fmt.Errorf("read Git status: %w", err)
	}
	changed := make(map[string]struct{})
	observedOmitted := make(map[string]struct{})
	statusPaths, err := parseGitPorcelainV1Z(status)
	if err != nil {
		return Snapshot{}, true, fmt.Errorf("parse Git status: %w", err)
	}
	for _, name := range statusPaths {
		if excludedContextPath(name) {
			if isEnvironmentPath(name) {
				observedOmitted[name] = struct{}{}
			}
			continue
		}
		changed[name] = struct{}{}
	}

	if hasHead {
		diff, err := runSnapshotGit(gitPath, absRoot,
			"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--name-only", "-z", "HEAD", "--")
		if err != nil {
			return Snapshot{}, true, fmt.Errorf("read Git changed path names: %w", err)
		}
		diffPaths, err := parseGitNameOnlyZ(diff)
		if err != nil {
			return Snapshot{}, true, fmt.Errorf("parse Git changed path names: %w", err)
		}
		for _, name := range diffPaths {
			if excludedContextPath(name) {
				if isEnvironmentPath(name) {
					observedOmitted[name] = struct{}{}
				}
				continue
			}
			changed[name] = struct{}{}
		}
	}

	explicit, err := snapshotFilesystem(absRoot, paths)
	if err != nil {
		return Snapshot{}, true, fmt.Errorf("snapshot explicit workspace paths: %w", err)
	}
	entries := make(map[string]SnapshotEntry, len(explicit.Entries)+len(changed))
	omittedPaths := make(map[string]struct{}, len(explicit.OmittedPaths)+len(observedOmitted))
	for name := range observedOmitted {
		omittedPaths[name] = struct{}{}
	}
	for _, name := range explicit.OmittedPaths {
		omittedPaths[name] = struct{}{}
	}
	for _, entry := range explicit.Entries {
		entries[entry.Path] = entry
	}

	rootHandle, err := os.OpenRoot(absRoot)
	if err != nil {
		return Snapshot{}, true, fmt.Errorf("open workspace for Git snapshot: %w", err)
	}
	defer rootHandle.Close()

	changedPaths := make([]string, 0, len(changed))
	for name := range changed {
		changedPaths = append(changedPaths, name)
	}
	sort.Strings(changedPaths)
	for _, name := range changedPaths {
		entry, included, err := snapshotFile(absRoot, rootHandle, name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				entries[name] = SnapshotEntry{Path: name}
				continue
			}
			return Snapshot{}, true, fmt.Errorf("snapshot changed Git path: %w", err)
		}
		if included {
			if entry.SHA256 == "" {
				omittedPaths[name] = struct{}{}
			} else {
				entries[name] = entry
			}
		}
	}

	result := Snapshot{
		Source:       "git",
		Entries:      make([]SnapshotEntry, 0, len(entries)),
		ChangedPaths: changedPaths,
		OmittedPaths: make([]string, 0, len(omittedPaths)),
	}
	for _, entry := range entries {
		result.Entries = append(result.Entries, entry)
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Path < result.Entries[j].Path })
	for omitted := range omittedPaths {
		result.OmittedPaths = append(result.OmittedPaths, omitted)
	}
	sort.Strings(result.OmittedPaths)
	return result, true, nil
}

func snapshotGitVersion(gitPath string) ([]byte, error) {
	cmd := exec.Command(gitPath, "--version")
	cmd.Env = snapshotGitEnvironment()
	return cmd.Output()
}

func supportsGitWorkspaceSnapshot(version []byte) bool {
	fields := strings.Fields(string(version))
	if len(fields) < 3 || fields[0] != "git" || fields[1] != "version" {
		return false
	}
	parts := strings.SplitN(fields[2], ".", 3)
	if len(parts) != 3 {
		return false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	_, patchDigits, errPatch := parseGitVersionPatch(parts[2])
	if errMajor != nil || errMinor != nil || errPatch != nil {
		return false
	}
	if patchDigits < len(parts[2]) {
		suffix := parts[2][patchDigits:]
		if (suffix[0] != '.' && suffix[0] != '+') || len(suffix) == 1 {
			return false
		}
		for _, part := range strings.Split(suffix[1:], ".") {
			if part == "" {
				return false
			}
			for _, char := range part {
				if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
					return false
				}
			}
		}
	}
	if major > 2 || major == 2 && minor > 36 {
		return true
	}
	if major != 2 || minor != 36 {
		return false
	}
	return true
}

func parseGitVersionPatch(value string) (int, int, error) {
	digits := 0
	for digits < len(value) && value[digits] >= '0' && value[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, 0, fmt.Errorf("invalid Git patch version")
	}
	patch, err := strconv.Atoi(value[:digits])
	return patch, digits, err
}

func runSnapshotGit(gitPath, root string, args ...string) ([]byte, error) {
	argv := []string{"--no-pager", "-c", "core.fsmonitor=false", "-C", root}
	argv = append(argv, args...)
	cmd := exec.Command(gitPath, argv...)
	cmd.Env = snapshotGitEnvironment()
	output, err := cmd.Output()
	if err != nil {
		command := "Git command"
		if len(args) > 0 {
			command = "Git " + args[0]
		}
		return nil, fmt.Errorf("%s failed: %w", command, err)
	}
	return output, nil
}

func snapshotGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if ok && !strings.HasPrefix(key, "GIT_") && key != "GCM_INTERACTIVE" {
			environment = append(environment, value)
		}
	}
	environment = append(environment,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	)
	return environment
}

func snapshotGitTopLevelMatches(root string, output []byte) bool {
	topLevel := string(output)
	if strings.HasSuffix(topLevel, "\n") {
		topLevel = strings.TrimSuffix(topLevel, "\n")
	}
	if topLevel == "" {
		return false
	}
	if !filepath.IsAbs(topLevel) {
		return false
	}
	topLevel = filepath.Clean(topLevel)
	root = filepath.Clean(root)
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false
	}
	topInfo, err := os.Stat(topLevel)
	if err != nil {
		return false
	}
	return os.SameFile(rootInfo, topInfo)
}

func parseGitPorcelainV1Z(output []byte) ([]string, error) {
	var paths []string
	for len(output) > 0 {
		separator := bytes.IndexByte(output, 0)
		if separator < 0 {
			return nil, fmt.Errorf("unterminated Git status record")
		}
		record := output[:separator]
		output = output[separator+1:]
		if len(record) < 4 || record[2] != ' ' {
			return nil, fmt.Errorf("invalid Git status record")
		}
		name := string(record[3:])
		if name == "" {
			return nil, fmt.Errorf("empty path in Git status record")
		}
		paths = append(paths, name)
		if record[0] == 'R' || record[0] == 'C' || record[1] == 'R' || record[1] == 'C' {
			separator = bytes.IndexByte(output, 0)
			if separator < 0 || separator == 0 {
				return nil, fmt.Errorf("invalid Git rename or copy record")
			}
			output = output[separator+1:]
		}
	}
	return paths, nil
}

func parseGitNameOnlyZ(output []byte) ([]string, error) {
	if len(output) == 0 {
		return nil, nil
	}
	var paths []string
	for len(output) > 0 {
		separator := bytes.IndexByte(output, 0)
		if separator < 0 {
			return nil, fmt.Errorf("unterminated Git path list")
		}
		if separator == 0 {
			return nil, fmt.Errorf("invalid Git path list")
		}
		paths = append(paths, string(output[:separator]))
		output = output[separator+1:]
	}
	return paths, nil
}
