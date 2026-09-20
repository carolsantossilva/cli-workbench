package workflows

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"station/internal/workspace"
)

func TestPrepareWorkflowBuildsAuditedContextWithoutPersistingInputContent(t *testing.T) {
	root := t.TempDir()
	manifest := workflowRunFixture([]string{"docs/station/idea.md"})
	inputContent := "A small shared calendar for families.\n"
	writeTestFile(t, root, "docs/station/idea.md", []byte(inputContent))
	writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("Brainstorm instruction v1."))

	packet, runID, err := prepareWorkflow(root, "brainstorm", Input{Manifest: manifest})
	if err != nil {
		t.Fatalf("prepareWorkflow() error = %v", err)
	}
	if len(runID) != 36 || packet.WorkflowID != "brainstorm" || packet.WorkspaceRoot != root {
		t.Fatalf("packet/runID = %#v / %q", packet, runID)
	}
	if packet.InstructionPath != "docs/station/workflows/brainstorm.md" || packet.Instruction != "Brainstorm instruction v1." {
		t.Fatalf("instruction = %#v", packet)
	}
	if packet.NextHandoff != "docs/station/brief.md" {
		t.Fatalf("next handoff = %q, want docs/station/brief.md", packet.NextHandoff)
	}
	if len(packet.Inputs) != 1 || packet.Inputs[0].Path != "docs/station/idea.md" || packet.Inputs[0].Content != inputContent {
		t.Fatalf("inputs = %#v, want the small approved input content", packet.Inputs)
	}
	packetJSON, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal context packet: %v", err)
	}
	if bytes.Contains(packetJSON, []byte(inputContent)) {
		t.Fatal("context packet JSON persisted full input content contrary to the JSON output contract")
	}
	if bytes.Contains(packetJSON, []byte("Brainstorm instruction v1.")) {
		t.Fatal("context packet JSON persisted full workflow instruction contrary to the JSON output contract")
	}
	wantHash := sha256.Sum256([]byte(inputContent))
	if packet.Inputs[0].SHA256 != hex.EncodeToString(wantHash[:]) || packet.Inputs[0].LineCount != 2 {
		t.Fatalf("input metadata = %#v, want content hash and line count", packet.Inputs[0])
	}
	if len(packet.ExpectedOutputs) != 1 || packet.ExpectedOutputs[0].Path != "docs/station/brief.md" || packet.ExpectedOutputs[0].Exists {
		t.Fatalf("expected outputs = %#v, want brief.md pending", packet.ExpectedOutputs)
	}

	logBytes, err := os.ReadFile(filepath.Join(root, ".station", "audit.jsonl"))
	if err != nil {
		t.Fatalf("read prepared audit: %v", err)
	}
	if bytes.Contains(logBytes, []byte(inputContent)) {
		t.Fatal("audit log persisted input content")
	}
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logBytes), &event); err != nil {
		t.Fatalf("prepared audit is not JSON: %v", err)
	}
	if event["kind"] != "prepared" || event["workflow"] != "brainstorm" || event["runId"] != runID {
		t.Fatalf("prepared audit metadata = %#v", event)
	}

	statePath := filepath.Join(root, ".station", "runs", runID+".json")
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read local run snapshot: %v", err)
	}
	if bytes.Contains(stateBytes, []byte(inputContent)) {
		t.Fatal("local run snapshot persisted input content")
	}
	if !bytes.Contains(stateBytes, []byte("docs/station/idea.md")) {
		t.Fatal("local run snapshot omits declared input path")
	}
}

func TestPrepareWorkflowMissingRequiredInputDoesNotCreateAudit(t *testing.T) {
	root := t.TempDir()
	manifest := workflowRunFixture([]string{"docs/station/idea.md"})
	writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("instruction"))

	_, runID, err := prepareWorkflow(root, "brainstorm", Input{Manifest: manifest})
	if err == nil || runID != "" {
		t.Fatalf("prepareWorkflow() = runID %q, err %v; want missing input error before run", runID, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(statErr) {
		t.Fatalf("missing input created .station: %v", statErr)
	}
}

func TestPrepareWorkflowRejectsMissingOutputContractBeforeCreatingRun(t *testing.T) {
	root := t.TempDir()
	manifest := workflowRunFixture(nil)
	workflow := manifest.Workflows["brainstorm"]
	workflow.Outputs = nil
	manifest.Workflows["brainstorm"] = workflow
	writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("instruction"))

	_, runID, err := PrepareWorkflow(root, "brainstorm", Input{Manifest: manifest})
	assertWorkflowFailureCode(t, err, FailureInvalid)
	if runID != "" {
		t.Fatalf("run ID = %q, want no prepared run without outputs", runID)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(statErr) {
		t.Fatalf("missing output contract created Station state: %v", statErr)
	}
}

func TestPrepareWorkflowExcludesSecretAndPEMInputContents(t *testing.T) {
	root := t.TempDir()
	manifest := workflowRunFixture([]string{"docs/station/.env.local", "docs/station/key.pem"})
	secret := "SERVICE_PASSWORD=fixture-private-value-must-not-appear"
	pem := "-----BEGIN PRIVATE KEY-----\nfixture-pem-value-must-not-appear\n-----END PRIVATE KEY-----\n"
	writeTestFile(t, root, "docs/station/.env.local", []byte(secret))
	writeTestFile(t, root, "docs/station/key.pem", []byte(pem))
	writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("instruction"))

	packet, runID, err := prepareWorkflow(root, "brainstorm", Input{Manifest: manifest})
	if err != nil {
		t.Fatalf("prepareWorkflow() error = %v", err)
	}
	if runID == "" || len(packet.Inputs) != 0 || len(packet.Omissions) != 2 {
		t.Fatalf("runID=%q inputs=%#v omissions=%#v", runID, packet.Inputs, packet.Omissions)
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal context packet: %v", err)
	}
	for _, raw := range []string{secret, "fixture-private-value-must-not-appear", pem, "fixture-pem-value-must-not-appear"} {
		if strings.Contains(string(encoded), raw) {
			t.Fatalf("context packet leaked secret material %q", raw)
		}
	}
	for _, path := range []string{"docs/station/.env.local", "docs/station/key.pem"} {
		if !bytes.Contains(encoded, []byte(path)) {
			t.Errorf("context packet omission metadata does not mention %q", path)
		}
	}
}

func TestPrepareWorkflowOmitsBinaryAndOversizedInputs(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{name: "binary", content: []byte{0x00, 0xff, 0x01, 0x02}},
		{name: "oversized", content: bytes.Repeat([]byte("x"), int(maxContextInputBytes+1))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := "docs/station/asset.bin"
			manifest := workflowRunFixture([]string{path})
			writeTestFile(t, root, path, test.content)
			writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("instruction"))

			packet, runID, err := prepareWorkflow(root, "brainstorm", Input{Manifest: manifest})
			if err != nil {
				t.Fatalf("prepareWorkflow() error = %v", err)
			}
			if runID == "" || len(packet.Inputs) != 0 || len(packet.Omissions) != 1 || packet.Omissions[0].Path != path {
				t.Fatalf("runID=%q inputs=%#v omissions=%#v", runID, packet.Inputs, packet.Omissions)
			}
		})
	}
}

func TestPrepareWorkflowOmitsInputSymlinkToEnvironmentFile(t *testing.T) {
	root := t.TempDir()
	secret := "SERVICE_PASSWORD=fixture-private-value-must-not-appear"
	writeTestFile(t, root, "docs/station/.env.local", []byte(secret))
	writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("safe instruction"))
	if err := os.Symlink(".env.local", filepath.Join(root, "docs", "station", "idea.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	packet, runID, err := prepareWorkflow(root, "brainstorm", Input{Manifest: workflowRunFixture([]string{"docs/station/idea.md"})})
	if err != nil {
		t.Fatalf("prepareWorkflow() error = %v", err)
	}
	if runID == "" || len(packet.Inputs) != 0 {
		t.Fatalf("runID=%q inputs=%#v; symlink target must not enter workflow inputs", runID, packet.Inputs)
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "fixture-private-value-must-not-appear") {
		t.Fatal("context packet leaked content through an input symlink")
	}
}

func TestPrepareWorkflowRejectsSymlinkedInstructionAndPEMInstruction(t *testing.T) {
	t.Run("symlink to environment file", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "docs/station/.env.local", []byte("SERVICE_PASSWORD=must-not-be-read"))
		instruction := filepath.Join(root, "docs", "station", "workflows", "brainstorm.md")
		if err := os.Symlink("../.env.local", instruction); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		_, runID, err := prepareWorkflow(root, "brainstorm", Input{Manifest: workflowRunFixture(nil)})
		assertWorkflowFailureCode(t, err, FailureDeny)
		if runID != "" {
			t.Fatalf("run ID = %q, want no prepared run", runID)
		}
		if _, err := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(err) {
			t.Fatalf("rejected instruction symlink created Station state: %v", err)
		}
	})
	t.Run("PEM secret content", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("-----BEGIN PRIVATE KEY-----\nprivate data\n-----END PRIVATE KEY-----\n"))
		_, runID, err := prepareWorkflow(root, "brainstorm", Input{Manifest: workflowRunFixture(nil)})
		assertWorkflowFailureCode(t, err, FailureDeny)
		if runID != "" {
			t.Fatalf("run ID = %q, want no prepared run", runID)
		}
		if _, err := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(err) {
			t.Fatalf("rejected secret instruction created Station state: %v", err)
		}
	})
}

func workflowRunFixture(inputs []string) *workspace.Workspace {
	return &workspace.Workspace{
		SchemaVersion: 1,
		Workspace:     workspace.Identity{ID: "workflow-fixture", Kind: "monorepo"},
		Projects:      []workspace.Project{},
		Commands:      []workspace.Command{},
		Policies: workspace.Policies{
			Execution:       map[string]any{},
			Data:            map[string]any{"categories": map[string]any{"design": "allow", "source": "allow"}},
			ExternalChanges: map[string]any{},
		},
		Workflows: map[string]workspace.Workflow{
			"brainstorm": {
				ReadPaths:  []string{"docs/station/**"},
				WritePaths: []string{"docs/station/**"},
				Inputs:     inputs,
				Outputs:    []string{"docs/station/brief.md"},
				Handoff:    "docs/station/brief.md",
			},
		},
		ExternalRepositories: []workspace.ExternalRepository{},
	}
}
