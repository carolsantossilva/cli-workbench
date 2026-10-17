package workflows

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"workbench/internal/workspace"
)

type runState struct {
	Version         int       `json:"version"`
	RunID           string    `json:"runId"`
	WorkflowID      string    `json:"workflowId"`
	PreparedEventID string    `json:"preparedEventId"`
	ManifestSHA256  string    `json:"manifestSha256"`
	Snapshot        Snapshot  `json:"snapshot"`
	OutputPaths     []string  `json:"outputPaths"`
	CreatedAt       time.Time `json:"createdAt"`
}

var errVerificationInProgress = errors.New("workflow verification is already in progress")

func acquireVerificationLock(root, runID string) (func(), error) {
	if !canonicalUUID(runID) {
		return nil, errors.New("invalid run ID")
	}
	runsRoot, err := openRunsRoot(root, false)
	if err != nil {
		return nil, err
	}
	filename := runID + ".verify-lock"
	file, err := runsRoot.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = runsRoot.Close()
		if errors.Is(err, fs.ErrExist) {
			return nil, errVerificationInProgress
		}
		return nil, fmt.Errorf("create verification lock: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = runsRoot.Remove(filename)
		_ = runsRoot.Close()
		return nil, fmt.Errorf("close verification lock: %w", err)
	}
	return func() {
		_ = runsRoot.Remove(filename)
		_ = runsRoot.Close()
	}, nil
}

func manifestFingerprint(manifest *workspace.Workspace) (string, error) {
	if manifest == nil {
		return "", errors.New("workspace manifest is required")
	}
	content, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("encode workspace manifest fingerprint: %w", err)
	}
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

// ValidatePreparedRunManifest rejects a policy/configuration change between
// workflow preparation and a subsequent command execution.
func ValidatePreparedRunManifest(root, workflowID, runID string, manifest *workspace.Workspace) error {
	state, err := readRunState(root, runID)
	if err != nil {
		return workflowFailure(FailurePrerequisiteMissing, ".workbench/runs", "prepared workflow run state is missing or not safely readable")
	}
	if !validRunStateForVerification(state, workflowID, runID) {
		return workflowFailure(FailureInvalid, ".workbench/runs", "prepared workflow run metadata does not match the requested workflow")
	}
	fingerprint, err := manifestFingerprint(manifest)
	if err != nil {
		return workflowFailure(FailureInvalid, "workspace.json", "workspace manifest is invalid")
	}
	if fingerprint != state.ManifestSHA256 {
		return workflowFailure(FailureInvalid, "workspace.json", "workspace contract changed after workflow preparation")
	}
	return nil
}

func writeRunState(root string, state runState) error {
	if !canonicalUUID(state.RunID) {
		return errors.New("invalid run ID")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode run state: %w", err)
	}

	runsRoot, err := openRunsRoot(root, true)
	if err != nil {
		return err
	}
	defer runsRoot.Close()

	file, err := runsRoot.OpenFile(state.RunID+".json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create run state exclusively: %w", err)
	}

	writeErr := file.Chmod(0o600)
	if writeErr == nil {
		var written int
		written, writeErr = file.Write(data)
		if writeErr == nil && written != len(data) {
			writeErr = io.ErrShortWrite
		}
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write run state: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close run state: %w", closeErr)
	}
	return nil
}

func readRunState(root, runID string) (runState, error) {
	if !canonicalUUID(runID) {
		return runState{}, errors.New("invalid run ID")
	}
	runsRoot, err := openRunsRoot(root, false)
	if err != nil {
		return runState{}, err
	}
	defer runsRoot.Close()

	filename := runID + ".json"
	info, err := runsRoot.Lstat(filename)
	if err != nil {
		return runState{}, fmt.Errorf("inspect run state: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return runState{}, errors.New("run state must be a regular file")
	}
	file, err := runsRoot.Open(filename)
	if err != nil {
		return runState{}, fmt.Errorf("open run state: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return runState{}, fmt.Errorf("stat run state: %w", err)
	}
	if !os.SameFile(info, openedInfo) {
		return runState{}, errors.New("run state changed while opening")
	}

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var state runState
	if err := decoder.Decode(&state); err != nil {
		return runState{}, fmt.Errorf("decode run state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return runState{}, errors.New("run state contains trailing JSON")
		}
		return runState{}, fmt.Errorf("decode trailing run state data: %w", err)
	}
	if state.RunID != runID {
		return runState{}, errors.New("run state ID does not match its file")
	}
	return state, nil
}

func openRunsRoot(root string, create bool) (*os.Root, error) {
	if root == "" {
		return nil, errors.New("workspace root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	workspaceRoot, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	defer workspaceRoot.Close()

	workbenchRoot, err := openStateDirectory(workspaceRoot, ".workbench", create)
	if err != nil {
		return nil, err
	}
	defer workbenchRoot.Close()

	return openStateDirectory(workbenchRoot, "runs", create)
}

func openStateDirectory(parent *os.Root, name string, create bool) (*os.Root, error) {
	created := false
	info, err := parent.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) && create {
		if err := parent.Mkdir(name, 0o700); err == nil {
			created = true
		} else if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create state directory: %w", err)
		}
		info, err = parent.Lstat(name)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect state directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("state path must be a directory, not a symlink or file")
	}

	directoryRoot, err := parent.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open state directory: %w", err)
	}
	openedInfo, err := directoryRoot.Stat(".")
	if err != nil {
		directoryRoot.Close()
		return nil, fmt.Errorf("stat opened state directory: %w", err)
	}
	if !os.SameFile(info, openedInfo) {
		directoryRoot.Close()
		return nil, errors.New("state directory changed while opening")
	}

	if created {
		directory, err := directoryRoot.Open(".")
		if err != nil {
			directoryRoot.Close()
			return nil, fmt.Errorf("open state directory handle: %w", err)
		}
		chmodErr := directory.Chmod(0o700)
		closeErr := directory.Close()
		if chmodErr != nil {
			directoryRoot.Close()
			return nil, fmt.Errorf("set private state directory permissions: %w", chmodErr)
		}
		if closeErr != nil {
			directoryRoot.Close()
			return nil, fmt.Errorf("close state directory handle: %w", closeErr)
		}
	}
	return directoryRoot, nil
}

func canonicalUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index := 0; index < len(value); index++ {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		character := value[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
