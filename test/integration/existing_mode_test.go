package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"station/internal/cli"
)

func TestInspectWithoutManifestIsShallowReadOnlyAndDoesNotRunProjectCommands(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "README.md", "# Metadata fixture\n")
	writeFixture(t, root, "go.mod", "module fixture.local/app\n")
	writeFixture(t, root, "package.json", `{"scripts":{"preinstall":"touch should-not-exist"}}`)
	writeFixture(t, root, "apps/mobile/src/private.ts", "private source sentinel\n")
	before := treeDigest(t, root)

	var stdout, stderr bytes.Buffer
	result := cli.Run(context.Background(), []string{"inspect", "--format", "json"}, cli.Dependencies{
		Root: root, Stdout: &stdout, Stderr: &stderr,
	})
	if !result.OK || result.ExitCode != 0 {
		t.Fatalf("station inspect failed: result=%#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("station inspect wrote to stderr: %q", stderr.String())
	}
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("inspect output is not JSON: %q: %v", stdout.String(), err)
	}
	if output["inspection"] == nil {
		t.Fatalf("inspect JSON does not include the inspection result: %#v", output)
	}
	if strings.Contains(stdout.String(), "private source sentinel") || strings.Contains(stdout.String(), "apps/mobile/src/private.ts") {
		t.Fatalf("shallow inspection exposed a recursive source file: %q", stdout.String())
	}
	if after := treeDigest(t, root); after != before {
		t.Fatalf("inspect changed the existing workspace: before=%x after=%x", before, after)
	}
	if _, err := os.Lstat(filepath.Join(root, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("inspect ran a project command from package metadata: %v", err)
	}
}

func TestInspectWithManifestLimitsEntriesToDeclaredReadPaths(t *testing.T) {
	root := t.TempDir()
	newResult := cli.Run(context.Background(), []string{"new", "fixture idea"}, cli.Dependencies{Root: root})
	if !newResult.OK {
		t.Fatalf("create inspection fixture: %#v", newResult)
	}
	writeFixture(t, root, "apps/web/src/private.ts", "outside declared read scope")
	before := treeDigest(t, root)

	var stdout, stderr bytes.Buffer
	result := cli.Run(context.Background(), []string{"inspect", "--format", "json"}, cli.Dependencies{
		Root: root, Stdout: &stdout, Stderr: &stderr,
	})
	if !result.OK || result.ExitCode != 0 {
		t.Fatalf("station inspect failed: result=%#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "docs/station/idea.md") {
		t.Fatalf("inspection omitted a declared read path: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "apps/web/src/private.ts") || strings.Contains(stdout.String(), "outside declared read scope") {
		t.Fatalf("inspection included a path outside readPaths: %q", stdout.String())
	}
	if after := treeDigest(t, root); after != before {
		t.Fatalf("inspect changed the existing workspace: before=%x after=%x", before, after)
	}
}

func TestInspectDoesNotFollowAllowlistedSymlinkOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secretPath := filepath.Join(outside, "README.md")
	const secret = "outside symlink target must not be read"
	if err := os.WriteFile(secretPath, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretPath, filepath.Join(root, "README.md")); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}

	var stdout, stderr bytes.Buffer
	result := cli.Run(context.Background(), []string{"inspect"}, cli.Dependencies{
		Root: root, Stdout: &stdout, Stderr: &stderr,
	})
	if !result.OK {
		t.Fatalf("station inspect failed: result=%#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), secret) {
		t.Fatalf("inspection followed an external symlink: %q", stdout.String())
	}
}

func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func treeDigest(t *testing.T, root string) [32]byte {
	t.Helper()
	hasher := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		_, _ = hasher.Write([]byte(filepath.ToSlash(relative) + "\x00"))
		info, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = hasher.Write([]byte(info.Mode().String() + "\x00"))
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = hasher.Write(content)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var digest [32]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}
