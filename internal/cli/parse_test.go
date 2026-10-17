package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestRunCLIRejectsUnplannedDefaultCompletionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"completion"}, Dependencies{Stdout: &stdout, Stderr: &stderr})

	if result.OK || result.ExitCode != 2 || !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("result = %#v, stdout = %q, stderr = %q; completion is not part of the planned command surface", result, stdout.String(), stderr.String())
	}
}

func TestRunCLIShowsVersion(t *testing.T) {
	var stdout bytes.Buffer
	result := runCLI(context.Background(), []string{"--version"}, Dependencies{Stdout: &stdout})

	if !result.OK || !strings.Contains(stdout.String(), "workbench") {
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
	if _, err := os.Stat(filepath.Join(root, ".workbench")); !os.IsNotExist(err) {
		t.Fatalf("validate created .workbench: %v", err)
	}
}

func TestRunCLIRejectsInvalidExecutionPolicy(t *testing.T) {
	root := writeValidWorkspace(t)
	manifestPath := filepath.Join(root, "workspace.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(content), `"execution": {}`, `"execution": {"default":"maybe"}`, 1)
	if updated == string(content) {
		t.Fatal("fixture did not contain the expected empty execution policy")
	}
	if err := os.WriteFile(manifestPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"validate", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if result.OK || result.Status != "invalid" || result.ExitCode != 2 {
		t.Fatalf("result = %#v, want invalid policy", result)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Path != "/policies/execution" {
		t.Fatalf("diagnostics = %#v, want execution policy path", result.Diagnostics)
	}
	if stderr.Len() != 0 || !strings.Contains(stdout.String(), "execution") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCLIRejectsSecretAllowInDataPolicy(t *testing.T) {
	root := writeValidWorkspace(t)
	manifestPath := filepath.Join(root, "workspace.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(content), `"data": {}`, `"data": {"categories":{"secret":"allow"}}`, 1)
	if updated == string(content) {
		t.Fatal("fixture did not contain the expected empty data policy")
	}
	if err := os.WriteFile(manifestPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"validate"}, Dependencies{Root: root, Stderr: &stderr})
	if result.OK || result.Status != "invalid" || result.ExitCode != 2 {
		t.Fatalf("result = %#v, want invalid data policy", result)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Path != "/policies/data" {
		t.Fatalf("diagnostics = %#v, want data policy path", result.Diagnostics)
	}
}

func TestRunCLIRejectsUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"unknown"}, Dependencies{Stderr: &stderr})

	if result.OK || result.ExitCode != 2 || !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("result = %#v, stderr = %q", result, stderr.String())
	}
}

func TestRunCLIValidateJSONUsesActualCommandSyntax(t *testing.T) {
	root := writeValidWorkspace(t)
	var stdout, stderr bytes.Buffer

	result := runCLI(context.Background(), []string{"validate", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})

	if !result.OK || result.Status != "ready" || result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	var output Result
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("stdout is not Result JSON: %q: %v", stdout.String(), err)
	}
	if !reflect.DeepEqual(output, result) || stderr.Len() != 0 {
		t.Fatalf("output = %#v, result = %#v, stderr = %q", output, result, stderr.String())
	}
}

func TestRunCLIValidateFailureEmitsJSONDiagnosticWithoutStderr(t *testing.T) {
	root := writeWorkspaceFixture(t, "workspace.invalid.json")
	var stdout, stderr bytes.Buffer

	result := runCLI(context.Background(), []string{"validate", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})

	if result.OK || result.Status != "invalid" || result.Code != string(CodeInvalid) || result.ExitCode != 2 {
		t.Fatalf("result = %#v", result)
	}
	var output Result
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("stdout is not Result JSON: %q: %v", stdout.String(), err)
	}
	if len(output.Diagnostics) != 1 || output.Diagnostics[0].Path != "/workspace/id" || output.Diagnostics[0].Severity != "error" || output.Diagnostics[0].Message == "" {
		t.Fatalf("output diagnostics = %#v", output.Diagnostics)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty in JSON mode", stderr.String())
	}
}

func TestRunCLIRejectsInvalidFormat(t *testing.T) {
	root := writeValidWorkspace(t)
	var stdout, stderr bytes.Buffer

	result := runCLI(context.Background(), []string{"validate", "--format", "yaml"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})

	if result.OK || result.Status != "invalid" || result.Code != string(CodeInvalid) || result.ExitCode != 2 {
		t.Fatalf("result = %#v", result)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid argument") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCLIWorkflowChildInheritsJSONFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer

	result := runCLI(context.Background(), []string{"workflow", "run", "build", "--format", "json"}, Dependencies{Stdout: &stdout, Stderr: &stderr})

	if result.OK || result.Status != "invalid" || result.Code != string(CodeInvalid) || result.ExitCode != 2 {
		t.Fatalf("result = %#v", result)
	}
	var output Result
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("stdout is not Result JSON: %q: %v", stdout.String(), err)
	}
	if !reflect.DeepEqual(output, result) || stderr.Len() != 0 {
		t.Fatalf("output = %#v, result = %#v, stderr = %q", output, result, stderr.String())
	}
}

func TestRunCLIEmitsArgumentValidationErrorsAsJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"workflow", "run", "--format", "json"}, Dependencies{Stdout: &stdout, Stderr: &stderr})
	if result.OK || result.Status != "invalid" || result.ExitCode != 2 {
		t.Fatalf("result = %#v", result)
	}
	var output Result
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("stdout is not JSON: %q: %v", stdout.String(), err)
	}
	if stderr.Len() != 0 || output.ExitCode != result.ExitCode {
		t.Fatalf("stdout=%q stderr=%q output=%#v", stdout.String(), stderr.String(), output)
	}
}

func writeValidWorkspace(t *testing.T) string {
	t.Helper()
	return writeWorkspaceFixture(t, "workspace.valid.json")
}

func writeWorkspaceFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
