package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"station/internal/workspace"
)

func TestWorkflowRunJSONIncludesPreparedContextAndRunID(t *testing.T) {
	root := writeWorkflowWorkspace(t, true)
	var stdout, stderr bytes.Buffer

	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if !result.OK || result.Status != "prepared" || result.ExitCode != 0 {
		t.Fatalf("result = %#v, stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("stdout is not JSON: %q: %v", stdout.String(), err)
	}
	run, ok := output["workflowRun"].(map[string]any)
	if !ok || run["runId"] == "" {
		t.Fatalf("workflowRun response missing run ID: %#v", output)
	}
	packet, ok := run["context"].(map[string]any)
	if !ok || packet["workflowId"] != "brainstorm" {
		t.Fatalf("workflow context missing: %#v", run)
	}
	inputs, ok := packet["inputs"].([]any)
	if !ok || len(inputs) != 1 || inputs[0].(map[string]any)["path"] != "docs/station/idea.md" {
		t.Fatalf("input metadata missing from prepared JSON: %#v", packet["inputs"])
	}
	if strings.Contains(stdout.String(), "fixture idea") {
		t.Fatal("JSON output included full input content contrary to the CLI output contract")
	}
	if strings.Contains(stdout.String(), "Brainstorm fixture instruction") {
		t.Fatal("JSON output included full workflow instruction contrary to the CLI output contract")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkflowRunTextIncludesInstructionsAndHandoff(t *testing.T) {
	root := writeWorkflowWorkspace(t, true)
	var stdout, stderr bytes.Buffer

	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if !result.OK || !strings.Contains(stdout.String(), "Brainstorm fixture instruction") || !strings.Contains(stdout.String(), "fixture idea for the CLI workflow") || !strings.Contains(stdout.String(), "docs/station/brief.md") {
		t.Fatalf("result=%#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkflowRunThenVerifyCompletesPreparedHandoff(t *testing.T) {
	root := writeWorkflowWorkspace(t, true)
	var runStdout, runStderr bytes.Buffer
	runResult := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "--format", "json"}, Dependencies{Root: root, Stdout: &runStdout, Stderr: &runStderr})
	if !runResult.OK {
		t.Fatalf("workflow run = %#v stdout=%q stderr=%q", runResult, runStdout.String(), runStderr.String())
	}
	var runResponse map[string]any
	if err := json.Unmarshal(runStdout.Bytes(), &runResponse); err != nil {
		t.Fatalf("decode workflow run: %v", err)
	}
	runID := runResponse["workflowRun"].(map[string]any)["runId"].(string)
	writeCLIWorkflowFile(t, root, "docs/station/brief.md", []byte("handed-off brief"))

	var verifyStdout, verifyStderr bytes.Buffer
	verifyResult := runCLI(context.Background(), []string{"workflow", "verify", "brainstorm", runID, "--format", "json"}, Dependencies{Root: root, Stdout: &verifyStdout, Stderr: &verifyStderr})
	if !verifyResult.OK || verifyResult.Status != "completed" || verifyResult.ExitCode != 0 {
		t.Fatalf("workflow verify = %#v stdout=%q stderr=%q", verifyResult, verifyStdout.String(), verifyStderr.String())
	}
	if !strings.Contains(verifyStdout.String(), "docs/station/brief.md") || !strings.Contains(verifyStdout.String(), "completed") {
		t.Fatal("workflow verification JSON omitted output metadata or completion status")
	}
}

func TestWorkflowRunMissingInputReturnsArtifactMissingWithoutAudit(t *testing.T) {
	root := writeWorkflowWorkspace(t, false)
	var stdout, stderr bytes.Buffer

	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if result.OK || result.Status != "blocked" || result.Code != string(CodeArtifactMissing) || result.ExitCode != 5 {
		t.Fatalf("result = %#v, stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(err) {
		t.Fatalf("failed preflight created audit state: %v", err)
	}
	if stderr.Len() != 0 || !strings.Contains(stdout.String(), "artifact-missing") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Path != "docs/station/idea.md" {
		t.Fatalf("missing-input diagnostics = %#v", result.Diagnostics)
	}
}

func TestWorkflowRunRejectsExtraArguments(t *testing.T) {
	root := writeWorkflowWorkspace(t, true)
	var stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "unexpected"}, Dependencies{Root: root, Stderr: &stderr})
	if result.OK || result.Code != string(CodeInvalid) || result.ExitCode != 2 {
		t.Fatalf("result = %#v, stderr=%q", result, stderr.String())
	}
}

func TestWorkflowCommandAskDoesNotStartProcess(t *testing.T) {
	touch, err := exec.LookPath("touch")
	if err != nil {
		t.Skip("touch executable is unavailable")
	}
	root := writeWorkflowWorkspace(t, true)
	updateWorkflowManifest(t, root, func(manifest *workspace.Workspace) {
		manifest.Projects = []workspace.Project{{ID: "local", Path: "docs/station", Kind: "server", Language: "go"}}
		manifest.Commands = []workspace.Command{{ID: "write-sentinel", Category: "test", Executable: touch, Args: []string{"sentinel"}, Project: "local", TimeoutSeconds: 5}}
	})
	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "--command", "write-sentinel", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if result.OK || result.Status != "ask" || result.ExitCode != 3 {
		auditData, _ := os.ReadFile(filepath.Join(root, ".station", "audit.jsonl"))
		t.Fatalf("result = %#v stdout=%q stderr=%q audit=%q", result, stdout.String(), stderr.String(), auditData)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "station", "sentinel")); !os.IsNotExist(err) {
		t.Fatalf("ask decision started command and created sentinel: %v", err)
	}
}

func TestWorkflowCommandNeverStartsShellEvenWhenListed(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	root := writeWorkflowWorkspace(t, true)
	updateWorkflowManifest(t, root, func(manifest *workspace.Workspace) {
		manifest.Projects = []workspace.Project{{ID: "local", Path: "docs/station", Kind: "server", Language: "go"}}
		manifest.Commands = []workspace.Command{{ID: "shell-write", Category: "test", Executable: shell, Args: []string{"-c", "touch sentinel"}, Project: "local", TimeoutSeconds: 5}}
		workflow := manifest.Workflows["brainstorm"]
		workflow.AllowedCommands = []string{"shell-write"}
		manifest.Workflows["brainstorm"] = workflow
	})
	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "--command", "shell-write", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if result.OK || result.Status != "blocked" || result.Code != string(CodeDeny) || result.ExitCode != 4 {
		t.Fatalf("result = %#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "station", "sentinel")); !os.IsNotExist(err) {
		t.Fatalf("shell command created a sentinel despite deny: %v", err)
	}
}

func TestWorkflowCommandRunsOnlyAllowedArgvAndAuditsHashes(t *testing.T) {
	printf, err := exec.LookPath("printf")
	if err != nil {
		t.Skip("printf executable is unavailable")
	}
	root := writeWorkflowWorkspace(t, true)
	updateWorkflowManifest(t, root, func(manifest *workspace.Workspace) {
		manifest.Projects = []workspace.Project{{ID: "local", Path: "docs/station", Kind: "server", Language: "go"}}
		manifest.Commands = []workspace.Command{{ID: "fixture-output", Category: "test", Executable: printf, Args: []string{"fixture command output"}, Project: "local", TimeoutSeconds: 5}}
		workflow := manifest.Workflows["brainstorm"]
		workflow.AllowedCommands = []string{"fixture-output"}
		manifest.Workflows["brainstorm"] = workflow
		manifest.Policies.Data["categories"].(map[string]any)["test-output"] = "allow"
	})
	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "--command", "fixture-output", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if !result.OK || result.Status != "prepared" || result.ExitCode != 0 {
		t.Fatalf("result = %#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "fixture command output") || !strings.Contains(stdout.String(), "stdoutSha256") {
		t.Fatalf("JSON must omit command output content but include its hash: %q", stdout.String())
	}
	auditData, err := os.ReadFile(filepath.Join(root, ".station", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(auditData, []byte("fixture command output")) {
		t.Fatal("audit persisted raw command output")
	}
	if !bytes.Contains(auditData, []byte(`"commandId":"fixture-output"`)) {
		t.Fatal("audit command events omitted the declared command ID")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkflowCommandOmitsTerminalControlOutput(t *testing.T) {
	printf, err := exec.LookPath("printf")
	if err != nil {
		t.Skip("printf executable is unavailable")
	}
	root := writeWorkflowWorkspace(t, true)
	updateWorkflowManifest(t, root, func(manifest *workspace.Workspace) {
		manifest.Projects = []workspace.Project{{ID: "local", Path: "docs/station", Kind: "server", Language: "go"}}
		manifest.Commands = []workspace.Command{{ID: "ansi-output", Category: "test", Executable: printf, Args: []string{"\x1b[2J"}, Project: "local", TimeoutSeconds: 5}}
		workflow := manifest.Workflows["brainstorm"]
		workflow.AllowedCommands = []string{"ansi-output"}
		manifest.Workflows["brainstorm"] = workflow
	})
	var stdout, stderr bytes.Buffer
	result := runCLI(context.Background(), []string{"workflow", "run", "brainstorm", "--command", "ansi-output", "--format", "json"}, Dependencies{Root: root, Stdout: &stdout, Stderr: &stderr})
	if !result.OK || !strings.Contains(stdout.String(), `"outputOmitted": true`) || bytes.Contains(stdout.Bytes(), []byte{0x1b}) {
		t.Fatalf("terminal-control command output was not safely omitted: %#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
}

func writeWorkflowWorkspace(t *testing.T, includeInput bool) string {
	t.Helper()
	root := t.TempDir()
	manifest := workspace.Workspace{
		SchemaVersion: 1,
		Workspace:     workspace.Identity{ID: "workflow-fixture", Kind: "monorepo"},
		Projects:      []workspace.Project{},
		Commands:      []workspace.Command{},
		Policies: workspace.Policies{
			Execution:       map[string]any{},
			Data:            map[string]any{"categories": map[string]any{"design": "allow", "source": "allow", "test-output": "allow"}},
			ExternalChanges: map[string]any{},
		},
		Workflows: map[string]workspace.Workflow{
			"brainstorm": {
				RequiredSkills:  []string{},
				AllowedSkills:   []string{},
				AllowedCommands: []string{},
				ReadPaths:       []string{"docs/station/**"},
				WritePaths:      []string{"docs/station/**"},
				Inputs:          []string{"docs/station/idea.md"},
				Outputs:         []string{"docs/station/brief.md"},
				Summaries:       []string{},
				Handoff:         "docs/station/brief.md",
			},
		},
		ExternalRepositories: []workspace.ExternalRepository{},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeCLIWorkflowFile(t, root, "workspace.json", encoded)
	writeCLIWorkflowFile(t, root, "docs/station/workflows/brainstorm.md", []byte("Brainstorm fixture instruction"))
	if includeInput {
		writeCLIWorkflowFile(t, root, "docs/station/idea.md", []byte("fixture idea for the CLI workflow"))
	}
	return root
}

func updateWorkflowManifest(t *testing.T, root string, update func(*workspace.Workspace)) {
	t.Helper()
	manifestPath := filepath.Join(root, "workspace.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest workspace.Workspace
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	update(&manifest)
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeCLIWorkflowFile(t *testing.T, root, name string, contents []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
