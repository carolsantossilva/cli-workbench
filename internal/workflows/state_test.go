package workflows

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const stateTestRunID = "8d0f90af-25e9-4c36-9a90-0e63e1d12001"

func TestRunStateRoundTripPersistsSnapshotMetadataOnly(t *testing.T) {
	root := t.TempDir()
	want := testRunState(stateTestRunID)

	if err := writeRunState(root, want); err != nil {
		t.Fatalf("write run state: %v", err)
	}

	got, err := readRunState(root, stateTestRunID)
	if err != nil {
		t.Fatalf("read run state: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}

	data, err := os.ReadFile(filepath.Join(root, ".workbench", "runs", stateTestRunID+".json"))
	if err != nil {
		t.Fatalf("read persisted JSON: %v", err)
	}
	if bytes.Contains(data, []byte(`"content"`)) {
		t.Fatalf("persisted run state contains content instead of snapshot metadata: %s", data)
	}
}

func TestWriteRunStateUsesPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	if err := writeRunState(root, testRunState(stateTestRunID)); err != nil {
		t.Fatalf("write run state: %v", err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(root, ".workbench"):                                 0o700,
		filepath.Join(root, ".workbench", "runs"):                         0o700,
		filepath.Join(root, ".workbench", "runs", stateTestRunID+".json"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("mode for %s = %#o, want %#o", path, got, want)
		}
	}
}

func TestWriteRunStatePreservesExistingWorkbenchDirectoryPermissions(t *testing.T) {
	root := t.TempDir()
	workbench := filepath.Join(root, ".workbench")
	if err := os.Mkdir(workbench, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(workbench, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeRunState(root, testRunState(stateTestRunID)); err != nil {
		t.Fatalf("write run state: %v", err)
	}
	info, err := os.Stat(workbench)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("pre-existing .workbench permissions changed to %#o, want 0755", got)
	}
	runsInfo, err := os.Stat(filepath.Join(workbench, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if got := runsInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("new runs directory permissions = %#o, want 0700", got)
	}
}

func TestWriteRunStateDoesNotOverwriteExistingRun(t *testing.T) {
	root := t.TempDir()
	first := testRunState(stateTestRunID)
	if err := writeRunState(root, first); err != nil {
		t.Fatalf("write initial run state: %v", err)
	}
	path := filepath.Join(root, ".workbench", "runs", stateTestRunID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read initial run state: %v", err)
	}

	second := first
	second.WorkflowID = "different-workflow"
	if err := writeRunState(root, second); err == nil {
		t.Fatal("second write unexpectedly succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read run state after duplicate write: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("duplicate write changed existing run state:\n before: %s\n after: %s", before, after)
	}
}

func TestMalformedRunIDIsRejectedBeforeCreatingWorkbenchDirectory(t *testing.T) {
	root := t.TempDir()
	badID := "../../private-run-id-input"
	state := testRunState(badID)
	if err := writeRunState(root, state); err == nil {
		t.Fatal("write unexpectedly accepted malformed run ID")
	} else if strings.Contains(err.Error(), badID) {
		t.Fatalf("error exposed malformed run ID: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".workbench")); !os.IsNotExist(err) {
		t.Fatalf(".workbench exists after malformed ID rejection: %v", err)
	}

	if _, err := readRunState(root, badID); err == nil {
		t.Fatal("read unexpectedly accepted malformed run ID")
	} else if strings.Contains(err.Error(), badID) {
		t.Fatalf("read error exposed malformed run ID: %v", err)
	}
}

func TestRunStateRejectsSymlinkedDirectoriesWithoutTouchingOutside(t *testing.T) {
	for _, name := range []string{".workbench", "runs"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "sentinel")
			const sentinelContents = "leave this outside file unchanged"
			if err := os.WriteFile(sentinel, []byte(sentinelContents), 0o600); err != nil {
				t.Fatalf("write outside sentinel: %v", err)
			}

			link := filepath.Join(root, ".workbench")
			if name == "runs" {
				if err := os.Mkdir(link, 0o700); err != nil {
					t.Fatalf("create .workbench: %v", err)
				}
				link = filepath.Join(link, "runs")
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatalf("create symlink: %v", err)
			}

			if err := writeRunState(root, testRunState(stateTestRunID)); err == nil {
				t.Fatal("write unexpectedly followed symlinked state directory")
			}
			if _, err := readRunState(root, stateTestRunID); err == nil {
				t.Fatal("read unexpectedly followed symlinked state directory")
			}
			got, err := os.ReadFile(sentinel)
			if err != nil {
				t.Fatalf("read outside sentinel: %v", err)
			}
			if string(got) != sentinelContents {
				t.Fatalf("outside sentinel changed: %q", got)
			}
			if _, err := os.Lstat(filepath.Join(outside, "runs", stateTestRunID+".json")); !os.IsNotExist(err) {
				t.Fatalf("run state was created outside workspace: %v", err)
			}
		})
	}
}

func TestRunStateRejectsNonDirectoryStatePaths(t *testing.T) {
	for _, name := range []string{".workbench", "runs"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".workbench")
			if name == "runs" {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("create .workbench: %v", err)
				}
				path = filepath.Join(path, "runs")
			}
			const existingContents = "not a directory"
			if err := os.WriteFile(path, []byte(existingContents), 0o600); err != nil {
				t.Fatalf("create non-directory path: %v", err)
			}

			if err := writeRunState(root, testRunState(stateTestRunID)); err == nil {
				t.Fatal("write unexpectedly accepted a non-directory state path")
			}
			if _, err := readRunState(root, stateTestRunID); err == nil {
				t.Fatal("read unexpectedly accepted a non-directory state path")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read existing file: %v", err)
			}
			if string(got) != existingContents {
				t.Fatalf("existing file changed: %q", got)
			}
		})
	}
}

func testRunState(runID string) runState {
	return runState{
		Version:         1,
		RunID:           runID,
		WorkflowID:      "implement",
		PreparedEventID: "26453614-8264-4979-8cf3-205208955ff2",
		ManifestSHA256:  strings.Repeat("b", 64),
		Snapshot: Snapshot{
			Source: "git",
			Entries: []SnapshotEntry{{
				Path:       "apps/web/src/main.go",
				Exists:     true,
				SHA256:     strings.Repeat("a", 64),
				Bytes:      128,
				ModifiedAt: time.Date(2026, time.September, 20, 12, 30, 0, 0, time.UTC),
			}},
			ChangedPaths: []string{"apps/web/src/main.go"},
		},
		OutputPaths: []string{"docs/workbench/plan.md"},
		CreatedAt:   time.Date(2026, time.September, 20, 12, 31, 0, 0, time.UTC),
	}
}
