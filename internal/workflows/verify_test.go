package workflows

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"station/internal/audit"
	"station/internal/workspace"
)

func TestVerifyWorkflowRecordsOutputMetadataAndPreparedHandoff(t *testing.T) {
	root, runID := prepareVerificationFixture(t)
	output := []byte("brief contents must not be persisted\n")
	writeTestFile(t, root, "docs/station/brief.md", output)

	result, err := VerifyWorkflow(root, "brainstorm", runID)
	if err != nil {
		t.Fatalf("VerifyWorkflow() error = %v", err)
	}
	if result.WorkflowID != "brainstorm" || result.RunID != runID || result.Status != "completed" {
		t.Fatalf("result identity/status = %#v", result)
	}
	wantHashBytes := sha256.Sum256(output)
	wantHash := hex.EncodeToString(wantHashBytes[:])
	if len(result.Outputs) != 1 || result.Outputs[0].Path != "docs/station/brief.md" || result.Outputs[0].HashAfter != wantHash || result.Outputs[0].Status != "complete" {
		t.Fatalf("outputs = %#v, want verified output hash %s", result.Outputs, wantHash)
	}

	logBytes, records := readVerificationAudit(t, root)
	if len(records) != 2 {
		t.Fatalf("audit record count = %d, want prepared + completed", len(records))
	}
	prepared, completed := records[0], records[1]
	if prepared.Event.Kind != audit.KindPrepared || completed.Event.Kind != audit.KindCompleted {
		t.Fatalf("audit kinds = %q, %q", prepared.Event.Kind, completed.Event.Kind)
	}
	if completed.Event.RelatedEventID != prepared.EventID || completed.Event.RunID != runID || completed.Event.Workflow != "brainstorm" {
		t.Fatalf("completion audit does not link to prepared run: %#v", completed)
	}
	if len(completed.Event.Paths) != 1 || completed.Event.Paths[0] != "docs/station/brief.md" || len(completed.Event.Hashes) != 1 || completed.Event.Hashes[0] != wantHash {
		t.Fatalf("completion audit metadata = %#v", completed.Event)
	}
	if bytes.Contains(logBytes, output) {
		t.Fatal("completion audit persisted output content")
	}
}

func TestVerifyWorkflowRejectsMissingDeclaredOutputWithoutCompletingRun(t *testing.T) {
	root, runID := prepareVerificationFixture(t)

	_, err := VerifyWorkflow(root, "brainstorm", runID)
	assertWorkflowFailureCode(t, err, FailureArtifactMissing)
	_, records := readVerificationAudit(t, root)
	if len(records) != 1 || records[0].Event.Kind != audit.KindPrepared {
		t.Fatalf("audit after missing output = %#v, want only prepared event", records)
	}
}

func TestVerifyWorkflowRejectsAlteredDeclaredSnapshotHashWithoutCompletingRun(t *testing.T) {
	root := t.TempDir()
	declaredPath := "docs/station/idea.md"
	writeTestFile(t, root, declaredPath, []byte("declared read input\n"))
	runID := prepareVerificationRun(t, root)

	state, err := readRunState(root, runID)
	if err != nil {
		t.Fatalf("read prepared run state: %v", err)
	}
	found := false
	for index := range state.Snapshot.Entries {
		if state.Snapshot.Entries[index].Path == declaredPath {
			if state.Snapshot.Entries[index].SHA256 == "" {
				t.Fatalf("prepared declared entry %q has no content hash", declaredPath)
			}
			state.Snapshot.Entries[index].SHA256 = ""
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("prepared snapshot omits declared read path %q", declaredPath)
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode altered run state: %v", err)
	}
	statePath := filepath.Join(root, ".station", "runs", runID+".json")
	if err := os.WriteFile(statePath, stateBytes, 0o600); err != nil {
		t.Fatalf("persist altered run state: %v", err)
	}
	writeTestFile(t, root, "docs/station/brief.md", []byte("verified output\n"))

	_, err = VerifyWorkflow(root, "brainstorm", runID)
	assertWorkflowFailureCode(t, err, FailureInvalid)
	_, records := readVerificationAudit(t, root)
	if len(records) != 1 || records[0].Event.Kind != audit.KindPrepared {
		t.Fatalf("audit after altered snapshot rejection = %#v, want only prepared event", records)
	}
}

func TestVerifyWorkflowRejectsMismatchedWorkflowIdentity(t *testing.T) {
	root, runID := prepareVerificationFixture(t)

	_, err := VerifyWorkflow(root, "review", runID)
	assertWorkflowFailureCode(t, err, FailureInvalid)
	_, records := readVerificationAudit(t, root)
	if len(records) != 1 {
		t.Fatalf("audit record count = %d, want only prepared event", len(records))
	}
}

func TestVerifyWorkflowRejectsInvalidRunIDWithoutCreatingState(t *testing.T) {
	root := t.TempDir()

	_, err := VerifyWorkflow(root, "brainstorm", "not-a-run-id")
	assertWorkflowFailureCode(t, err, FailureInvalid)
	if _, statErr := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid run ID created Station state: %v", statErr)
	}
}

func TestVerifyWorkflowReportsObservableChangesOutsideWritePaths(t *testing.T) {
	root := t.TempDir()
	path := "apps/web/notes.md"
	writeTestFile(t, root, path, []byte("old note\n"))
	runID := prepareVerificationRun(t, root)
	state, err := readRunState(root, runID)
	if err != nil {
		t.Fatalf("read prepared run state: %v", err)
	}
	entry, ok := snapshotEntryMap(state.Snapshot.Entries)[path]
	if !ok || entry.SHA256 != "" || entry.Bytes != int64(len("old note\n")) || entry.ModifiedAt.IsZero() {
		t.Fatalf("out-of-scope baseline entry = %#v, want metadata and no content hash", entry)
	}
	writeTestFile(t, root, "docs/station/brief.md", []byte("verified brief\n"))
	writeTestFile(t, root, path, []byte("new note\n"))
	changedAt := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(path)), changedAt, changedAt); err != nil {
		t.Fatalf("set changed out-of-scope mtime: %v", err)
	}

	result, err := VerifyWorkflow(root, "brainstorm", runID)
	if err != nil {
		t.Fatalf("VerifyWorkflow() error = %v", err)
	}
	if len(result.OutOfScopeChanges) != 1 || result.OutOfScopeChanges[0] != path {
		t.Fatalf("out-of-scope changes = %#v, want %s", result.OutOfScopeChanges, path)
	}
	if !strings.Contains(strings.Join(result.Warnings, " "), "Git path-name evidence may not distinguish edits to paths already changed at preparation") {
		t.Fatalf("verification warnings omit the filesystem observability limitation: %#v", result.Warnings)
	}
}

func TestVerifyWorkflowReportsNewBinaryPathWithoutSnapshottingItsContents(t *testing.T) {
	root := t.TempDir()
	runID := prepareVerificationRun(t, root)
	writeTestFile(t, root, "docs/station/brief.md", []byte("verified brief\n"))
	writeTestFile(t, root, "apps/web/image.bin", []byte{0x00, 0xff, 0x11})

	result, err := VerifyWorkflow(root, "brainstorm", runID)
	if err != nil {
		t.Fatalf("VerifyWorkflow() error = %v", err)
	}
	if len(result.OutOfScopeChanges) != 1 || result.OutOfScopeChanges[0] != "apps/web/image.bin" {
		t.Fatalf("out-of-scope changes = %#v, want binary path metadata", result.OutOfScopeChanges)
	}
}

func TestVerifyWorkflowRejectsReusingCompletedRun(t *testing.T) {
	root, runID := prepareVerificationFixture(t)
	writeTestFile(t, root, "docs/station/brief.md", []byte("brief\n"))
	if _, err := VerifyWorkflow(root, "brainstorm", runID); err != nil {
		t.Fatalf("first VerifyWorkflow() error = %v", err)
	}

	_, err := VerifyWorkflow(root, "brainstorm", runID)
	assertWorkflowFailureCode(t, err, FailureInvalid)
	_, records := readVerificationAudit(t, root)
	if len(records) != 2 {
		t.Fatalf("audit record count after reuse = %d, want exactly one completion", len(records))
	}
}

func TestVerificationLockSerializesTheSameRun(t *testing.T) {
	root := t.TempDir()
	if err := writeRunState(root, testRunState(stateTestRunID)); err != nil {
		t.Fatal(err)
	}
	release, err := acquireVerificationLock(root, stateTestRunID)
	if err != nil {
		t.Fatalf("acquire first verification lock: %v", err)
	}
	if _, err := acquireVerificationLock(root, stateTestRunID); !errors.Is(err, errVerificationInProgress) {
		t.Fatalf("second verification lock error = %v, want in-progress", err)
	}
	release()
	releaseAgain, err := acquireVerificationLock(root, stateTestRunID)
	if err != nil {
		t.Fatalf("acquire released verification lock: %v", err)
	}
	releaseAgain()
}

func TestVerifyWorkflowRejectsMalformedAuditWithoutExposingRecord(t *testing.T) {
	root, runID := prepareVerificationFixture(t)
	writeTestFile(t, root, "docs/station/brief.md", []byte("brief\n"))
	const sentinel = "private-value-in-malformed-audit-record"
	auditPath := filepath.Join(root, ".station", "audit.jsonl")
	file, err := os.OpenFile(auditPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	if _, err := file.WriteString(sentinel + "\n"); err != nil {
		file.Close()
		t.Fatalf("append malformed audit record: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close audit log: %v", err)
	}

	_, err = VerifyWorkflow(root, "brainstorm", runID)
	if err == nil {
		t.Fatal("VerifyWorkflow() accepted malformed audit record")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("error exposed raw audit record: %v", err)
	}
	logBytes, readErr := os.ReadFile(auditPath)
	if readErr != nil {
		t.Fatalf("read audit log after rejection: %v", readErr)
	}
	if got := bytes.Count(logBytes, []byte("\n")); got != 2 {
		t.Fatalf("audit line count after rejection = %d, want prepared + malformed input only", got)
	}
}

func TestVerifyWorkflowRejectsUnsafeAuditPathMetadata(t *testing.T) {
	root, runID := prepareVerificationFixture(t)
	writeTestFile(t, root, "docs/station/brief.md", []byte("brief\n"))
	const unsafePath = "Authorization: fixture-private-value"
	record := verificationAuditRecord{
		EventID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Event: audit.Event{
			Kind:     audit.KindDecision,
			Decision: audit.DecisionAllow,
			Paths:    []string{unsafePath},
		},
	}
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("encode unsafe fixture audit record: %v", err)
	}
	auditPath := filepath.Join(root, ".station", "audit.jsonl")
	file, err := os.OpenFile(auditPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		t.Fatalf("append unsafe audit record: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close audit log: %v", err)
	}

	_, err = VerifyWorkflow(root, "brainstorm", runID)
	assertWorkflowFailureCode(t, err, FailureDeny)
	if strings.Contains(err.Error(), "fixture-private-value") {
		t.Fatalf("error exposed unsafe audit metadata: %v", err)
	}
}

func TestVerifyWorkflowRejectsSymlinkedAuditWithoutTouchingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires platform symlink support")
	}
	root, runID := prepareVerificationFixture(t)
	writeTestFile(t, root, "docs/station/brief.md", []byte("brief\n"))
	auditPath := filepath.Join(root, ".station", "audit.jsonl")
	if err := os.Remove(auditPath); err != nil {
		t.Fatalf("remove local audit file: %v", err)
	}
	outside := t.TempDir()
	target := filepath.Join(outside, "audit.jsonl")
	const original = "outside audit data stays untouched\n"
	if err := os.WriteFile(target, []byte(original), 0o600); err != nil {
		t.Fatalf("write outside audit target: %v", err)
	}
	if err := os.Symlink(target, auditPath); err != nil {
		t.Fatalf("create audit symlink: %v", err)
	}

	_, err := VerifyWorkflow(root, "brainstorm", runID)
	assertWorkflowFailureCode(t, err, FailureDeny)
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read outside audit target: %v", readErr)
	}
	if string(got) != original {
		t.Fatalf("outside audit target changed: %q", got)
	}
}

func TestVerifyWorkflowRejectsManifestChangedAfterPreparation(t *testing.T) {
	root, runID := prepareVerificationFixture(t)
	writeTestFile(t, root, "docs/station/brief.md", []byte("brief\n"))
	writeTestFile(t, root, "workspace.json", []byte("{ not valid workspace json"))

	_, err := VerifyWorkflow(root, "brainstorm", runID)
	assertWorkflowFailureCode(t, err, FailureInvalid)
	_, records := readVerificationAudit(t, root)
	if len(records) != 1 {
		t.Fatalf("invalid workspace manifest created a completion event: %#v", records)
	}
}

func TestVerifyWorkflowRejectsExpandedWriteScopeAfterPreparation(t *testing.T) {
	root, runID := prepareVerificationFixture(t)
	writeTestFile(t, root, "docs/station/brief.md", []byte("brief\n"))
	writeTestFile(t, root, "apps/web/notes.md", []byte("out-of-scope change\n"))

	manifestPath := filepath.Join(root, "workspace.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest workspace.Workspace
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	workflow := manifest.Workflows["brainstorm"]
	workflow.WritePaths = append(workflow.WritePaths, "apps/web/**")
	manifest.Workflows["brainstorm"] = workflow
	manifestBytes, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "workspace.json", manifestBytes)

	_, err = VerifyWorkflow(root, "brainstorm", runID)
	assertWorkflowFailureCode(t, err, FailureInvalid)
	_, records := readVerificationAudit(t, root)
	if len(records) != 1 || records[0].Event.Kind != audit.KindPrepared {
		t.Fatalf("expanded policy emitted completion audit event: %#v", records)
	}
}

func prepareVerificationFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	return root, prepareVerificationRun(t, root)
}

func prepareVerificationRun(t *testing.T, root string) string {
	t.Helper()
	manifest := workflowRunFixture(nil)
	brainstorm := manifest.Workflows["brainstorm"]
	brainstorm.RequiredSkills = []string{}
	brainstorm.AllowedSkills = []string{}
	brainstorm.AllowedCommands = []string{}
	brainstorm.Inputs = []string{}
	brainstorm.Summaries = []string{}
	manifest.Workflows["brainstorm"] = brainstorm
	review := brainstorm
	manifest.Workflows["review"] = review
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal workflow fixture: %v", err)
	}
	writeTestFile(t, root, "workspace.json", manifestBytes)
	writeTestFile(t, root, "docs/station/workflows/brainstorm.md", []byte("versioned brainstorm instruction\n"))
	_, runID, err := PrepareWorkflow(root, "brainstorm", Input{})
	if err != nil {
		t.Fatalf("PrepareWorkflow() fixture: %v", err)
	}
	return runID
}

type verificationAuditRecord struct {
	EventID   string `json:"eventId"`
	Timestamp string `json:"timestamp"`
	audit.Event
}

func readVerificationAudit(t *testing.T, root string) ([]byte, []verificationAuditRecord) {
	t.Helper()
	logBytes, err := os.ReadFile(filepath.Join(root, ".station", "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var records []verificationAuditRecord
	for _, line := range bytes.Split(bytes.TrimSpace(logBytes), []byte("\n")) {
		var value verificationAuditRecord
		if err := json.Unmarshal(line, &value); err != nil {
			t.Fatalf("decode test audit record: %v", err)
		}
		records = append(records, value)
	}
	return logBytes, records
}

func assertWorkflowFailureCode(t *testing.T, err error, want FailureCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected workflow failure %q, got nil", want)
	}
	var workflowErr *WorkflowError
	if !errors.As(err, &workflowErr) {
		t.Fatalf("error %T = %v, want *WorkflowError", err, err)
	}
	if workflowErr.Code != want {
		t.Fatalf("failure code = %q, want %q (error: %v)", workflowErr.Code, want, err)
	}
}
