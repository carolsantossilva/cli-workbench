package workflows

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSnapshotGitWorkspaceFallsBackWithoutInitializingRepository(t *testing.T) {
	root := t.TempDir()
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly has .git before snapshot: %v", err)
	}

	snapshot, usedGit, err := snapshotGitWorkspace(root, nil)
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if usedGit {
		t.Fatal("snapshotGitWorkspace() selected Git outside a repository")
	}
	if snapshot.Source != "" || len(snapshot.Entries) != 0 {
		t.Fatalf("fallback snapshot = %#v, want zero snapshot for caller fallback", snapshot)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("snapshotGitWorkspace initialized a repository: %v", err)
	}
}

func TestSnapshotGitWorkspaceFallsBackForNestedDirectoryOfParentRepository(t *testing.T) {
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	nested := filepath.Join(root, "apps", "web")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested workspace: %v", err)
	}

	snapshot, usedGit, err := snapshotGitWorkspace(nested, nil)
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if usedGit || snapshot.Source != "" {
		t.Fatalf("snapshotGitWorkspace() = (%#v, %t), want filesystem fallback", snapshot, usedGit)
	}
}

func TestSnapshotGitWorkspaceCapturesChangesAndExplicitPaths(t *testing.T) {
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	writeGitFixture(t, root, "tracked.txt", []byte("before\n"))
	writeGitFixture(t, root, "deleted.txt", []byte("remove me\n"))
	writeGitFixture(t, root, "stable/brief.md", []byte("explicit input\n"))
	writeGitFixture(t, root, "secret.pem", []byte("-----BEGIN PRIVATE KEY-----\nprivate fixture\n-----END PRIVATE KEY-----\n"))
	writeGitFixture(t, root, ".env.local", []byte("SERVICE_PASSWORD=private fixture\n"))
	runSnapshotTestGit(t, root, "add", "--", "tracked.txt", "deleted.txt", "stable/brief.md")
	runSnapshotTestGit(t, root, "-c", "user.name=Station Test", "-c", "user.email=station@example.invalid", "commit", "--quiet", "-m", "baseline")

	trackedContent := []byte("after\n")
	untrackedContent := []byte("new file\n")
	writeGitFixture(t, root, "tracked.txt", trackedContent)
	if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
		t.Fatalf("delete tracked fixture: %v", err)
	}
	writeGitFixture(t, root, "new file.txt", untrackedContent)

	snapshot, usedGit, err := snapshotGitWorkspace(root, []string{"stable/brief.md", "secret.pem"})
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if !usedGit || snapshot.Source != "git" {
		t.Fatalf("snapshotGitWorkspace() = (%#v, %t), want Git snapshot", snapshot, usedGit)
	}
	wantChanged := []string{"deleted.txt", "new file.txt", "secret.pem", "tracked.txt"}
	if fmt.Sprint(snapshot.ChangedPaths) != fmt.Sprint(wantChanged) {
		t.Fatalf("ChangedPaths = %#v, want %#v", snapshot.ChangedPaths, wantChanged)
	}
	wantEntries := map[string]SnapshotEntry{
		"deleted.txt":     {Path: "deleted.txt", Exists: false},
		"new file.txt":    {Path: "new file.txt", Exists: true, Bytes: int64(len(untrackedContent))},
		"stable/brief.md": gitEntry(t, "stable/brief.md", []byte("explicit input\n")),
		"tracked.txt":     {Path: "tracked.txt", Exists: true, Bytes: int64(len(trackedContent))},
	}
	if len(snapshot.Entries) != len(wantEntries) {
		t.Fatalf("Entries = %#v, want entries for changed files and explicit path", snapshot.Entries)
	}
	for index, entry := range snapshot.Entries {
		if index > 0 && snapshot.Entries[index-1].Path >= entry.Path {
			t.Fatalf("Entries are not unique and sorted: %#v", snapshot.Entries)
		}
		want, ok := wantEntries[entry.Path]
		if !ok {
			t.Errorf("unexpected snapshot entry %#v", entry)
			continue
		}
		if entry.Path != want.Path || entry.Exists != want.Exists || entry.SHA256 != want.SHA256 || entry.Bytes != want.Bytes {
			t.Errorf("entry %q = %#v, want %#v", entry.Path, entry, want)
		}
	}
	if fmt.Sprint(snapshot.OmittedPaths) != fmt.Sprint([]string{".env.local", "secret.pem"}) {
		t.Fatalf("OmittedPaths = %#v, want environment and PEM paths without file content", snapshot.OmittedPaths)
	}
}

func TestSnapshotGitWorkspaceReportsOutOfScopePathWithoutHashingContent(t *testing.T) {
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	declaredContent := []byte("declared input\n")
	writeGitFixture(t, root, "declared.md", declaredContent)
	runSnapshotTestGit(t, root, "add", "--", "declared.md")
	runSnapshotTestGit(t, root, "-c", "user.name=Station Test", "-c", "user.email=station@example.invalid", "commit", "--quiet", "-m", "baseline")

	outsideContent := []byte("out-of-scope content that must not be hashed or persisted\n")
	writeGitFixture(t, root, "outside.md", outsideContent)
	snapshot, usedGit, err := snapshotGitWorkspace(root, []string{"declared.md"})
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if !usedGit || snapshot.Source != "git" {
		t.Fatalf("snapshotGitWorkspace() = (%#v, %t), want Git snapshot", snapshot, usedGit)
	}
	if len(snapshot.ChangedPaths) != 1 || snapshot.ChangedPaths[0] != "outside.md" {
		t.Fatalf("ChangedPaths = %#v, want outside.md", snapshot.ChangedPaths)
	}
	entries := snapshotEntryMap(snapshot.Entries)
	declared, ok := entries["declared.md"]
	if !ok || declared.SHA256 != gitHash(declaredContent) {
		t.Fatalf("declared entry = %#v, want its declared content hash", declared)
	}
	outside, ok := entries["outside.md"]
	if !ok || outside.SHA256 != "" || outside.Bytes != int64(len(outsideContent)) || outside.ModifiedAt.IsZero() {
		t.Fatalf("out-of-scope Git entry = %#v, want metadata without a content hash", outside)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal Git snapshot: %v", err)
	}
	if bytes.Contains(encoded, outsideContent) {
		t.Fatal("Git snapshot persisted out-of-scope content")
	}
}

func TestSnapshotGitWorkspaceCapturesUntrackedFilesWithoutHEAD(t *testing.T) {
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	content := []byte("first commit has not happened yet\n")
	writeGitFixture(t, root, "draft.md", content)

	snapshot, usedGit, err := snapshotGitWorkspace(root, nil)
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if !usedGit || snapshot.Source != "git" {
		t.Fatalf("snapshotGitWorkspace() = (%#v, %t), want Git snapshot for unborn repository", snapshot, usedGit)
	}
	if len(snapshot.ChangedPaths) != 1 || snapshot.ChangedPaths[0] != "draft.md" {
		t.Fatalf("ChangedPaths = %#v, want draft.md", snapshot.ChangedPaths)
	}
	if len(snapshot.Entries) != 1 || snapshot.Entries[0].Path != "draft.md" || snapshot.Entries[0].SHA256 != "" || snapshot.Entries[0].Bytes != int64(len(content)) {
		t.Fatalf("Entries = %#v, want metadata-only entry for undeclared draft.md", snapshot.Entries)
	}
}

func TestSnapshotGitWorkspaceReturnsErrorsAfterRecognizingSameRootRepository(t *testing.T) {
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	writeGitFixture(t, root, "tracked.txt", []byte("baseline\n"))
	runSnapshotTestGit(t, root, "add", "--", "tracked.txt")
	runSnapshotTestGit(t, root, "-c", "user.name=Station Test", "-c", "user.email=station@example.invalid", "commit", "--quiet", "-m", "baseline")

	index := filepath.Join(root, ".git", "index")
	if err := os.Remove(index); err != nil {
		t.Fatalf("remove Git index fixture: %v", err)
	}
	if err := os.Mkdir(index, 0o700); err != nil {
		t.Fatalf("replace Git index with invalid directory: %v", err)
	}

	_, usedGit, err := snapshotGitWorkspace(root, nil)
	if !usedGit || err == nil {
		t.Fatalf("snapshotGitWorkspace() = (usedGit %t, error %v), want detected Git repository and an error", usedGit, err)
	}
}

func TestSnapshotGitWorkspacePreservesNewlineAndWhitespaceInPaths(t *testing.T) {
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	name := "folder with space/line\nbreak\tname.txt"
	content := []byte("untracked content\n")
	writeGitFixture(t, root, name, content)

	snapshot, usedGit, err := snapshotGitWorkspace(root, nil)
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if !usedGit {
		t.Fatal("snapshotGitWorkspace() did not select the supported Git adapter")
	}
	if len(snapshot.ChangedPaths) != 1 || snapshot.ChangedPaths[0] != name {
		t.Fatalf("ChangedPaths = %#v, want exact NUL-delimited filename %q", snapshot.ChangedPaths, name)
	}
	if len(snapshot.Entries) != 1 || snapshot.Entries[0].Path != name || snapshot.Entries[0].SHA256 != "" || snapshot.Entries[0].Bytes != int64(len(content)) {
		t.Fatalf("Entries = %#v, want metadata-only entry for exact filename %q", snapshot.Entries, name)
	}
}

func TestSnapshotGitWorkspaceDisablesConfiguredFSMonitorHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture fsmonitor hook uses a local POSIX executable")
	}
	root := t.TempDir()
	initSnapshotTestRepo(t, root)
	writeGitFixture(t, root, "tracked.txt", []byte("baseline\n"))
	runSnapshotTestGit(t, root, "add", "--", "tracked.txt")
	runSnapshotTestGit(t, root, "-c", "user.name=Station Test", "-c", "user.email=station@example.invalid", "commit", "--quiet", "-m", "baseline")

	marker := filepath.Join(root, "fsmonitor-ran")
	hook := filepath.Join(root, "fsmonitor-hook")
	script := "#!/bin/sh\nprintf called > " + shellQuoteTestPath(marker) + "\nprintf '\n'\n"
	if err := os.WriteFile(hook, []byte(script), 0o700); err != nil {
		t.Fatalf("write fsmonitor hook fixture: %v", err)
	}
	runSnapshotTestGit(t, root, "config", "core.fsmonitor", hook)

	snapshot, usedGit, err := snapshotGitWorkspace(root, nil)
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if !usedGit || snapshot.Source != "git" {
		t.Fatalf("snapshotGitWorkspace() = (%#v, %t), want Git snapshot", snapshot, usedGit)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("configured fsmonitor hook ran during snapshot: %v", err)
	}
}

func TestSnapshotGitWorkspaceFallsBackWhenGitIsUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("empty PATH setup is platform-specific")
	}
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)

	snapshot, usedGit, err := snapshotGitWorkspace(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if usedGit || snapshot.Source != "" {
		t.Fatalf("snapshotGitWorkspace() = (%#v, %t), want fallback without Git", snapshot, usedGit)
	}
}

func TestSnapshotGitWorkspaceFallsBackForOldGitVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake Git executable fixture uses a local POSIX executable")
	}
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	git := filepath.Join(bin, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shellQuoteTestPath(calls) + "\nprintf 'git version 2.35.9\\n'\n"
	if err := os.WriteFile(git, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake Git executable: %v", err)
	}
	t.Setenv("PATH", bin)

	snapshot, usedGit, err := snapshotGitWorkspace(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("snapshotGitWorkspace() error = %v", err)
	}
	if usedGit || snapshot.Source != "" {
		t.Fatalf("snapshotGitWorkspace() = (%#v, %t), want filesystem fallback for Git 2.35.9", snapshot, usedGit)
	}
	callLog, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("read fake Git call log: %v", err)
	}
	if string(callLog) != "--version\n" {
		t.Fatalf("Git calls = %q, want only version detection before fallback", callLog)
	}
}

func TestSupportsGitWorkspaceSnapshotUsesVersionBoundary(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "git version 2.35.99", want: false},
		{version: "git version 2.36.0", want: true},
		{version: "git version 2.36.0.windows.1", want: true},
		{version: "git version 2.36.0-rc0", want: false},
		{version: "git version 2.36.0invalid", want: false},
		{version: "git version 2.36.0.", want: false},
		{version: "git version 2.36.0+", want: false},
		{version: "git version 2.36.0..1", want: false},
		{version: "git version 2.36.1", want: true},
		{version: "git version 3.0.0", want: true},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			if got := supportsGitWorkspaceSnapshot([]byte(test.version)); got != test.want {
				t.Fatalf("supportsGitWorkspaceSnapshot(%q) = %t, want %t", test.version, got, test.want)
			}
		})
	}
}

func TestParseGitNameOnlyZRejectsMalformedLists(t *testing.T) {
	for _, output := range [][]byte{{0}, {'a', 0, 0}, {'a'}} {
		if paths, err := parseGitNameOnlyZ(output); err == nil {
			t.Errorf("parseGitNameOnlyZ(%q) = %#v, nil error; want malformed list rejected", output, paths)
		}
	}
	paths, err := parseGitNameOnlyZ([]byte("one\x00two\x00"))
	if err != nil || fmt.Sprint(paths) != "[one two]" {
		t.Fatalf("parseGitNameOnlyZ(valid list) = %#v, %v", paths, err)
	}
}

func initSnapshotTestRepo(t *testing.T, root string) {
	t.Helper()
	if !snapshotTestGitIsSupported(t) {
		t.Skip("Git 2.36.0 or newer is required for this adapter integration test")
	}
	runSnapshotTestGit(t, root, "init", "--quiet")
}

func snapshotTestGitIsSupported(t *testing.T) bool {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	output, err := exec.Command(path, "--version").Output()
	if err != nil {
		return false
	}
	var major, minor, patch int
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(string(output)), "git version "), "%d.%d.%d", &major, &minor, &patch); err != nil {
		return false
	}
	return major > 2 || major == 2 && (minor > 36 || minor == 36 && patch >= 0)
}

func runSnapshotTestGit(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	argv := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", argv...)
	cmd.Env = snapshotGitTestEnvironment(root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return output
}

func snapshotGitTestEnvironment(root string) []string {
	values := map[string]string{
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_GLOBAL":   filepath.Join(root, "global-gitconfig"),
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_OPTIONAL_LOCKS":  "0",
		"GIT_NO_LAZY_FETCH":   "1",
	}
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if _, replaced := values[key]; !replaced || !ok {
			env = append(env, value)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func writeGitFixture(t *testing.T, root, name string, content []byte) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatalf("create parent for %q: %v", name, err)
	}
	if err := os.WriteFile(absolute, content, 0o600); err != nil {
		t.Fatalf("write %q: %v", name, err)
	}
}

func gitEntry(t *testing.T, name string, content []byte) SnapshotEntry {
	t.Helper()
	return SnapshotEntry{Path: name, Exists: true, SHA256: gitHash(content), Bytes: int64(len(content))}
}

func gitHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func shellQuoteTestPath(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
