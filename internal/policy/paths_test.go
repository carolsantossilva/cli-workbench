package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssertWorkspacePathAcceptsLocalMissingWriteTarget(t *testing.T) {
	root := t.TempDir()
	got, rule, err := assertWorkspacePath(root, "docs/new.md", PathWrite)
	if err != nil {
		t.Fatalf("assertWorkspacePath() error = %v", err)
	}
	if got != filepath.Join(root, "docs", "new.md") {
		t.Fatalf("path = %q, want path below root", got)
	}
	if rule == "" {
		t.Fatal("matched rule is empty")
	}
}

func TestAssertWorkspacePathRejectsEmptyRoot(t *testing.T) {
	if _, _, err := assertWorkspacePath("", "docs/new.md", PathWrite); err == nil {
		t.Fatal("assertWorkspacePath with empty root succeeded")
	}
}

func TestAssertWorkspacePathRejectsLexicalEscapes(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"", ".", "../outside", "a/../../outside", "a/./b", "a//b", "a/", filepath.Join(string(filepath.Separator), "etc", "passwd")} {
		t.Run(strings.ReplaceAll(path, string(filepath.Separator), "_"), func(t *testing.T) {
			if _, _, err := assertWorkspacePath(root, path, PathRead); err == nil {
				t.Fatalf("assertWorkspacePath(%q) unexpectedly succeeded", path)
			}
		})
	}
}

func TestGoRootRejectsFinalExternalSymlinkWithTrailingSlash(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	anchored, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer anchored.Close()

	if _, err := anchored.Open("link/"); err == nil {
		t.Fatal("os.Root.Open followed a final external symlink with trailing slash")
	}
}

func TestAssertWorkspacePathRejectsSymlinkOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	for _, path := range []string{"link/secret.txt", "link/"} {
		if _, _, err := assertWorkspacePath(root, path, PathRead); err == nil {
			t.Errorf("assertWorkspacePath(%q) unexpectedly followed external symlink", path)
		}
	}
}

func TestAssertWorkspacePathSupportsExternalReadMode(t *testing.T) {
	root := t.TempDir()
	if _, _, err := assertWorkspacePath(root, "docs/reference.md", PathExternalRead); err != nil {
		t.Fatalf("external read path error = %v", err)
	}
}
