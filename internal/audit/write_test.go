package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendAddsJSONLLinesWithoutRewritingPriorEvents(t *testing.T) {
	root := t.TempDir()
	first := Event{
		Kind:       KindPrepared,
		Decision:   DecisionAllow,
		RunID:      "run-1",
		Workflow:   "build",
		Paths:      []string{"apps/web/src/main.tsx"},
		Categories: []string{"source"},
		Hashes:     []string{strings.Repeat("a", 64)},
	}
	firstID, auditPath, err := Append(root, first)
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	firstBytes, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read first audit record: %v", err)
	}
	if !strings.HasSuffix(string(firstBytes), "\n") {
		t.Fatalf("first record is not newline-terminated: %q", firstBytes)
	}

	secondID, secondPath, err := Append(root, Event{
		Kind:           KindCompleted,
		Decision:       DecisionAllow,
		RunID:          "run-1",
		Workflow:       "build",
		RelatedEventID: firstID,
		Categories:     []string{"source"},
	})
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if secondPath != auditPath {
		t.Fatalf("second path = %q, want %q", secondPath, auditPath)
	}
	if firstID == secondID {
		t.Fatalf("Append returned duplicate event ID %q", firstID)
	}

	allBytes, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read appended audit records: %v", err)
	}
	if !bytes.HasPrefix(allBytes, firstBytes) {
		t.Fatalf("append changed the first record: first=%q all=%q", firstBytes, allBytes)
	}
	if !strings.HasSuffix(string(allBytes), "\n") {
		t.Fatalf("appended records are not newline-terminated: %q", allBytes)
	}

	lines := strings.Split(strings.TrimSuffix(string(allBytes), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSONL line count = %d, want 2: %q", len(lines), allBytes)
	}
	gotIDs := make(map[string]bool, len(lines))
	for i, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("line %d is not valid JSON: %q", i+1, line)
		}
		var record struct {
			EventID   string    `json:"eventId"`
			Timestamp time.Time `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode line %d: %v", i+1, err)
		}
		if record.EventID == "" || record.Timestamp.IsZero() || record.Timestamp.Location() != time.UTC {
			t.Fatalf("line %d metadata = %#v, want event ID and UTC timestamp", i+1, record)
		}
		gotIDs[record.EventID] = true
	}
	if len(gotIDs) != 2 || !gotIDs[firstID] || !gotIDs[secondID] {
		t.Fatalf("event IDs in JSONL = %#v, want %q and %q", gotIDs, firstID, secondID)
	}

	for _, path := range []string{filepath.Dir(auditPath), auditPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %q: %v", path, err)
		}
		if got := info.Mode().Perm() & 0o077; got != 0 {
			t.Errorf("permissions for %q expose group/world bits: %04o", path, info.Mode().Perm())
		}
	}
}

func TestAppendPersistsDeclaredCommandIDForCommandEvent(t *testing.T) {
	root := t.TempDir()
	_, auditPath, err := Append(root, Event{
		Kind: KindCommand, Decision: DecisionAllow, RunID: "e2a62b62-17a3-4fab-a68d-f8b710d0e56e",
		Workflow: "build", CommandID: "server.test", Paths: []string{"apps/server"},
	})
	if err != nil {
		t.Fatalf("Append(command) error = %v", err)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"commandId":"server.test"`)) {
		t.Fatalf("audit record omitted command ID: %s", data)
	}
}

func TestAppendRejectsInvalidOrUndecidedEventWithoutCreatingAuditDirectory(t *testing.T) {
	rawSecret := "fixture-secret-value-must-not-appear"
	for _, test := range []struct {
		name     string
		decision Decision
	}{
		{name: "undecided", decision: ""},
		{name: "unknown decision", decision: "maybe"},
		{name: "secret-like invalid value", decision: Decision(rawSecret)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			eventID, auditPath, err := Append(root, Event{Kind: KindDecision, Decision: test.decision})
			if err == nil {
				t.Fatalf("Append with decision %q succeeded", test.name)
			}
			if eventID != "" || auditPath != "" {
				t.Fatalf("Append returned eventID=%q path=%q on error", eventID, auditPath)
			}
			if strings.Contains(err.Error(), rawSecret) {
				t.Fatal("validation error leaked raw input")
			}
			if _, err := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(err) {
				t.Fatalf("invalid event created .station: stat error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, ".station", "audit.jsonl")); !os.IsNotExist(err) {
				t.Fatalf("invalid event created audit file: stat error = %v", err)
			}
		})
	}
}

func TestAppendRejectsValueLikeAuditPathsWithoutPersistingThem(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "credential assignment", value: "SERVICE_PASSWORD=fixture-private-value"},
		{name: "secret-labeled value", value: "fixture-secret-value-must-not-appear"},
		{name: "PEM marker", value: "-----BEGIN PRIVATE KEY-----"},
		{name: "bearer credential", value: "Authorization: Bearer ghp_fixture_token_0123456789"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			_, _, err := Append(root, Event{
				Kind:     KindPrepared,
				Decision: DecisionAllow,
				Paths:    []string{test.value},
			})
			if err == nil {
				t.Fatal("Append accepted value-like path metadata")
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatal("validation error leaked rejected audit metadata")
			}
			if _, err := os.Stat(filepath.Join(root, ".station")); !os.IsNotExist(err) {
				t.Fatalf("rejected metadata created .station: stat error = %v", err)
			}
		})
	}
}

func TestAppendPreservesSensitiveLookingFilenamesAsPaths(t *testing.T) {
	for _, value := range []string{"docs/secret-value.md", "docs/private-key.md"} {
		t.Run(value, func(t *testing.T) {
			root := t.TempDir()
			_, auditPath, err := Append(root, Event{
				Kind:     KindPrepared,
				Decision: DecisionAllow,
				Paths:    []string{value},
			})
			if err != nil {
				t.Fatalf("Append(valid path %q) error = %v", value, err)
			}
			encoded, err := os.ReadFile(auditPath)
			if err != nil {
				t.Fatalf("read audit record: %v", err)
			}
			if !bytes.Contains(encoded, []byte(value)) {
				t.Fatalf("audit record omits valid path %q: %s", value, encoded)
			}
		})
	}
}

func TestAppendRejectsEmptyRootWithoutWritingToWorkingDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	t.Chdir(workingDirectory)

	if _, _, err := Append("", Event{Kind: KindDecision, Decision: DecisionAllow}); err == nil {
		t.Fatal("Append with an empty root succeeded")
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, ".station")); !os.IsNotExist(err) {
		t.Fatalf("empty root created .station in the working directory: stat error = %v", err)
	}
}

func TestAppendRejectsSymlinkEscapes(t *testing.T) {
	validEvent := Event{Kind: KindDecision, Decision: DecisionAllow}

	t.Run("station directory", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, ".station")); err != nil {
			t.Fatalf("create .station symlink: %v", err)
		}

		if _, _, err := Append(root, validEvent); err == nil {
			t.Fatal("Append through escaping .station symlink succeeded")
		}
		if _, err := os.Stat(filepath.Join(outside, "audit.jsonl")); !os.IsNotExist(err) {
			t.Fatalf("outside audit file was created: stat error = %v", err)
		}
	})

	t.Run("audit file", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.jsonl")
		if err := os.WriteFile(outside, []byte("leave-this-file-alone\n"), 0o600); err != nil {
			t.Fatalf("create outside sentinel: %v", err)
		}
		if err := os.Mkdir(filepath.Join(root, ".station"), 0o700); err != nil {
			t.Fatalf("create .station: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".station", "audit.jsonl")); err != nil {
			t.Fatalf("create audit symlink: %v", err)
		}

		if _, _, err := Append(root, validEvent); err == nil {
			t.Fatal("Append through escaping audit-file symlink succeeded")
		}
		got, err := os.ReadFile(outside)
		if err != nil {
			t.Fatalf("read outside sentinel: %v", err)
		}
		if string(got) != "leave-this-file-alone\n" {
			t.Fatalf("outside sentinel changed to %q", got)
		}
	})
}

func TestAppendWritesOnlyEncodedRedactedMetadata(t *testing.T) {
	root := t.TempDir()
	rawSecret := "fixture-secret-value-must-not-appear"
	secretHash := sha256.Sum256([]byte(rawSecret))
	event := Event{
		Kind:       KindPrepared,
		Decision:   DecisionAllow,
		RunID:      "run-2",
		Workflow:   "review",
		Paths:      []string{"config/[REDACTED]"},
		Categories: []string{"secret"},
		Hashes:     []string{hex.EncodeToString(secretHash[:])},
	}
	_, auditPath, err := Append(root, event)
	if err != nil {
		t.Fatalf("Append(redacted event) error = %v", err)
	}
	encoded, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit record: %v", err)
	}
	if bytes.Contains(encoded, []byte(rawSecret)) {
		t.Fatal("audit contains raw secret content")
	}

	line := bytes.TrimSuffix(encoded, []byte("\n"))
	var record map[string]json.RawMessage
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatalf("audit line is not JSON: %v", err)
	}
	for _, field := range []string{"eventId", "timestamp", "kind", "decision", "runId", "workflow", "paths", "categories", "hashes"} {
		if _, ok := record[field]; !ok {
			t.Errorf("encoded event is missing %q: %s", field, line)
		}
	}
	for field := range record {
		switch field {
		case "eventId", "timestamp", "kind", "decision", "runId", "workflow", "paths", "categories", "hashes", "relatedEventId":
		default:
			t.Errorf("encoded event contains unapproved field %q", field)
		}
	}
	if !bytes.Contains(encoded, []byte(hex.EncodeToString(secretHash[:]))) {
		t.Errorf("audit omits redacted SHA-256 fingerprint: %s", encoded)
	}
}
