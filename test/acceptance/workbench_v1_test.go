package acceptance_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/internal/cli"
	"workbench/internal/workspace"
)

func TestWorkbenchV1NewValidateAcceptanceDoesNotModifyItsSourceOrGenerateAnApp(t *testing.T) {
	sourceRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore := acceptanceTreeSnapshot(t, sourceRoot)
	workspaceRoot := t.TempDir()

	newResult, newStdout, newStderr := runCLI(t, workspaceRoot, "new", "uma ideia de aplicação")
	if !newResult.OK || newResult.ExitCode != 0 || newStderr != "" {
		t.Fatalf("workbench new result = %#v, stdout=%q, stderr=%q", newResult, newStdout, newStderr)
	}
	if got, want := readAcceptanceFile(t, workspaceRoot, "docs/workbench/idea.md"), "uma ideia de aplicação\n"; got != want {
		t.Fatalf("stored idea = %q, want %q", got, want)
	}
	if _, err := workspace.LoadWorkspace(workspaceRoot); err != nil {
		t.Fatalf("generated manifest is invalid: %v", err)
	}

	for _, relative := range []string{
		"workspace.json",
		"schemas/workspace.schema.json",
		"docs/workbench/idea.md",
		"README.md",
		"AGENTS.md",
		"apps/mobile/README.md",
		"apps/mobile/AGENTS.md",
		"apps/web/README.md",
		"apps/web/AGENTS.md",
		"apps/server/README.md",
		"apps/server/AGENTS.md",
		"docs/workbench/workflows/brainstorm.md",
		"docs/workbench/workflows/grill-me.md",
		"docs/workbench/workflows/design.md",
		"docs/workbench/workflows/build.md",
		"docs/workbench/workflows/review.md",
		".agents/skills/workbench-mobile/SKILL.md",
		".agents/skills/workbench-web/SKILL.md",
		".agents/skills/workbench-server/SKILL.md",
	} {
		info, err := os.Stat(filepath.Join(workspaceRoot, filepath.FromSlash(relative)))
		if err != nil || !info.Mode().IsRegular() {
			t.Errorf("acceptance workspace is missing %s: %v", relative, err)
		}
	}

	for _, forbidden := range []string{
		".git",
		"apps/mobile/package.json",
		"apps/web/package.json",
		"apps/server/go.mod",
		"apps/mobile/src",
		"apps/web/src",
		"apps/server/cmd",
		"packages/shared",
		"node_modules",
		"package-lock.json",
	} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, filepath.FromSlash(forbidden))); !os.IsNotExist(err) {
			t.Errorf("acceptance workspace unexpectedly contains %s: %v", forbidden, err)
		}
	}
	if err := filepath.WalkDir(filepath.Join(workspaceRoot, "apps"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(filepath.Join(workspaceRoot, "apps"), path)
		if err != nil {
			return err
		}
		if filepath.Base(relative) != "AGENTS.md" && filepath.Base(relative) != "README.md" {
			return &unexpectedAppFile{path: relative}
		}
		return nil
	}); err != nil {
		t.Fatalf("application scaffold contains product files: %v", err)
	}

	validateResult, validateStdout, validateStderr := runCLI(t, workspaceRoot, "validate")
	if !validateResult.OK || validateResult.ExitCode != 0 || validateStderr != "" {
		t.Fatalf("workbench validate result = %#v, stdout=%q, stderr=%q", validateResult, validateStdout, validateStderr)
	}
	if !strings.Contains(validateStdout, "valid") {
		t.Fatalf("workbench validate did not confirm the manifest: %q", validateStdout)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".workbench")); !os.IsNotExist(err) {
		t.Errorf("new/validate created runtime audit state unexpectedly: %v", err)
	}
	if got := acceptanceTreeSnapshot(t, sourceRoot); !equalAcceptanceSnapshots(got, sourceBefore) {
		t.Fatalf("acceptance changed its source directory:\nbefore=%v\nafter=%v", sourceBefore, got)
	}
}

type unexpectedAppFile struct{ path string }

func (err *unexpectedAppFile) Error() string { return "unexpected app file " + err.path }

func runCLI(t *testing.T, root string, args ...string) (cli.Result, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	result := cli.Run(context.Background(), args, cli.Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	return result, stdout.String(), stderr.String()
}

func readAcceptanceFile(t *testing.T, root, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(content)
}

func acceptanceTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	values := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.IsDir() {
			values[filepath.ToSlash(relative)] = "directory"
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			values[filepath.ToSlash(relative)] = "symlink:" + target
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		values[filepath.ToSlash(relative)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot source %s: %v", root, err)
	}
	return values
}

func equalAcceptanceSnapshots(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for path, value := range left {
		if right[path] != value {
			return false
		}
	}
	return true
}
