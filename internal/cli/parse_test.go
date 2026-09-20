package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLIShowsRootHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"--help"}, Dependencies{Stdout: &stdout, Stderr: &stderr})

	if !result.OK || !strings.Contains(stdout.String(), "validate") || stderr.Len() != 0 {
		t.Fatalf("result = %#v, stdout = %q, stderr = %q", result, stdout.String(), stderr.String())
	}
}

func TestRunCLIShowsVersion(t *testing.T) {
	var stdout bytes.Buffer
	result := runCLI(context.Background(), []string{"--version"}, Dependencies{Stdout: &stdout})

	if !result.OK || !strings.Contains(stdout.String(), "station") {
		t.Fatalf("result = %#v, stdout = %q", result, stdout.String())
	}
}

func TestRunCLIShowsWorkflowHelp(t *testing.T) {
	var stdout bytes.Buffer
	result := runCLI(context.Background(), []string{"workflow", "--help"}, Dependencies{Stdout: &stdout})

	if !result.OK || !strings.Contains(stdout.String(), "run") || !strings.Contains(stdout.String(), "verify") {
		t.Fatalf("result = %#v, stdout = %q", result, stdout.String())
	}
}

func TestRunCLIValidatesManifestWithoutWriting(t *testing.T) {
	root := writeValidWorkspace(t)
	var stdout bytes.Buffer
	result := runCLI(context.Background(), []string{"validate"}, Dependencies{Root: root, Stdout: &stdout})

	if !result.OK || result.ExitCode != 0 || !strings.Contains(stdout.String(), "valid") {
		t.Fatalf("result = %#v, stdout = %q", result, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(err) {
		t.Fatalf("validate created .station: %v", err)
	}
}

func TestRunCLIRejectsUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"unknown"}, Dependencies{Stderr: &stderr})

	if result.OK || result.ExitCode != 2 || !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("result = %#v, stderr = %q", result, stderr.String())
	}
}

func writeValidWorkspace(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "workspace.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
