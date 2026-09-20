package workflows

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"station/internal/audit"
	"station/internal/policy"
	"station/internal/workspace"
)

const maxAuditRecordBytes = 1 << 20

type auditLogRecord struct {
	EventID   string `json:"eventId"`
	Timestamp string `json:"timestamp"`
	audit.Event
}

// VerifyWorkflow checks a prepared run's outputs and records a metadata-only
// completion event linked to its prepared audit event.
func VerifyWorkflow(root, workflowID, runID string) (WorkflowResult, error) {
	if strings.TrimSpace(root) == "" || workflowID == "" || !validRunID(runID) {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "", "workflow ID, run ID, and workspace root are required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "", "could not resolve workspace root")
	}
	releaseLock, err := acquireVerificationLock(absoluteRoot, runID)
	if err != nil {
		if errors.Is(err, errVerificationInProgress) {
			return WorkflowResult{}, workflowFailure(FailureAsk, ".station/runs/"+runID+".verify-lock", "workflow verification is already in progress or a prior attempt left a stale lock")
		}
		return WorkflowResult{}, workflowFailure(FailurePrerequisiteMissing, ".station/runs", "could not safely acquire workflow verification lock")
	}
	defer releaseLock()

	state, err := readRunState(absoluteRoot, runID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return WorkflowResult{}, workflowFailure(FailurePrerequisiteMissing, ".station/runs", "prepared workflow run state is missing")
		}
		return WorkflowResult{}, workflowFailure(FailureDeny, ".station/runs", "prepared workflow run state is not safely readable")
	}
	if !validRunStateForVerification(state, workflowID, runID) {
		return WorkflowResult{}, workflowFailure(FailureInvalid, ".station/runs", "prepared workflow run metadata does not match the requested workflow")
	}

	if err := validateWorkspaceManifestFile(absoluteRoot); err != nil {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "workspace.json", "workspace manifest is missing or not a regular local file")
	}
	manifest, err := workflowManifest(absoluteRoot, nil)
	if err != nil {
		return WorkflowResult{}, err
	}
	if err := validateWorkflowManifest(manifest, absoluteRoot); err != nil {
		return WorkflowResult{}, err
	}
	manifestSHA256, err := manifestFingerprint(manifest)
	if err != nil || manifestSHA256 != state.ManifestSHA256 {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "workspace.json", "workspace contract changed after workflow preparation")
	}
	resolved, err := Resolve(workflowID, manifest)
	if err != nil {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "/workflows/"+workflowID, "workflow is not valid in the workspace manifest")
	}
	if len(resolved.Configuration.Outputs) == 0 {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "/workflows/"+workflowID+"/outputs", "workflow outputs must be declared")
	}
	if !samePathSet(state.OutputPaths, resolved.Configuration.Outputs) {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "/workflows/"+workflowID+"/outputs", "workflow output configuration changed after preparation")
	}

	records, err := readAuditLog(absoluteRoot)
	if err != nil {
		return WorkflowResult{}, workflowFailure(FailureDeny, ".station/audit.jsonl", "workflow audit log is malformed or not safely readable")
	}
	prepared, completed, err := findRunAuditEvents(records, state, workflowID, runID)
	if err != nil {
		return WorkflowResult{}, err
	}
	if completed {
		return WorkflowResult{}, workflowFailure(FailureInvalid, ".station/audit.jsonl", "workflow run has already been completed")
	}
	if prepared == nil {
		return WorkflowResult{}, workflowFailure(FailurePrerequisiteMissing, ".station/audit.jsonl", "prepared workflow audit event is missing")
	}
	if !snapshotHashesMatchPreparedEvent(state.Snapshot, resolved.Configuration, prepared.Event) {
		return WorkflowResult{}, workflowFailure(FailureInvalid, ".station/runs", "prepared workflow snapshot hashes do not match prepared audit metadata")
	}

	paths := workflowSnapshotPaths(resolved.Configuration)
	currentSnapshot, err := snapshotWorkspace(absoluteRoot, paths)
	if err != nil {
		return WorkflowResult{}, workflowFailure(FailureDeny, "", "could not safely snapshot workspace for verification")
	}
	if currentSnapshot.Source != state.Snapshot.Source {
		return WorkflowResult{}, workflowFailure(FailureInvalid, "", "workspace snapshot source changed since workflow preparation")
	}
	comparisonSnapshot := currentSnapshot
	if currentSnapshot.Source == "filesystem" {
		metadataSnapshot, snapshotErr := snapshotFilesystemMetadata(absoluteRoot)
		if snapshotErr != nil {
			return WorkflowResult{}, workflowFailure(FailureDeny, "", "could not safely compare workspace metadata")
		}
		comparisonSnapshot = mergeDeclaredSnapshot(metadataSnapshot, currentSnapshot, paths)
	}

	outputs, err := verifyDeclaredOutputs(absoluteRoot, resolved.Configuration, manifest, workflowID, state.Snapshot, currentSnapshot)
	if err != nil {
		return WorkflowResult{}, err
	}
	outOfScope := changedPathsOutsideWriteScope(state.Snapshot, comparisonSnapshot, resolved.Configuration.WritePaths)

	eventPaths := make([]string, 0, len(outputs))
	eventHashes := make([]string, 0, len(outputs))
	for _, output := range outputs {
		eventPaths = append(eventPaths, output.Path)
		eventHashes = append(eventHashes, output.HashAfter)
	}
	_, _, err = audit.Append(absoluteRoot, audit.Event{
		Kind:           audit.KindCompleted,
		Decision:       audit.DecisionAllow,
		RunID:          runID,
		Workflow:       workflowID,
		Paths:          eventPaths,
		Hashes:         eventHashes,
		RelatedEventID: state.PreparedEventID,
	})
	if err != nil {
		return WorkflowResult{}, workflowFailure(FailurePrerequisiteMissing, ".station/audit.jsonl", "could not append workflow completion audit event")
	}

	result := WorkflowResult{
		WorkflowID:        workflowID,
		RunID:             runID,
		Status:            "completed",
		Outputs:           outputs,
		OutOfScopeChanges: outOfScope,
	}
	if len(outOfScope) > 0 {
		result.Warnings = []string{
			"Observed changes outside workflow write paths are reported, not prevented by Station.",
			"Filesystem edits preserving both size and modification time may be missed without Git evidence. Git path-name evidence may not distinguish edits to paths already changed at preparation.",
		}
	}
	return result, nil
}

func validateWorkspaceManifestFile(root string) error {
	workspaceRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer workspaceRoot.Close()

	info, err := workspaceRoot.Lstat("workspace.json")
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("workspace manifest is not a regular file")
	}
	file, err := workspaceRoot.Open("workspace.json")
	if err != nil {
		return err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, openedInfo) {
		return errors.New("workspace manifest changed while opening")
	}
	return nil
}

func validRunStateForVerification(state runState, workflowID, runID string) bool {
	if state.Version != 1 || state.RunID != runID || state.WorkflowID != workflowID || !canonicalUUID(state.PreparedEventID) || !validSHA256(state.ManifestSHA256) || state.CreatedAt.IsZero() {
		return false
	}
	if state.Snapshot.Source != "filesystem" && state.Snapshot.Source != "git" {
		return false
	}
	if len(state.OutputPaths) == 0 {
		return false
	}
	for _, output := range state.OutputPaths {
		if !validSnapshotPattern(output) {
			return false
		}
	}
	if !validSnapshotEntries(state.Snapshot.Entries) || !validSnapshotPathList(state.Snapshot.ChangedPaths) || !validSnapshotPathList(state.Snapshot.OmittedPaths) {
		return false
	}
	return true
}

func validSnapshotEntries(entries []SnapshotEntry) bool {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !validSnapshotPattern(entry.Path) || entry.Bytes < 0 {
			return false
		}
		if _, exists := seen[entry.Path]; exists {
			return false
		}
		seen[entry.Path] = struct{}{}
		if entry.Exists {
			if (entry.SHA256 != "" && !validSHA256(entry.SHA256)) || entry.ModifiedAt.IsZero() {
				return false
			}
		} else if entry.SHA256 != "" || entry.Bytes != 0 || !entry.ModifiedAt.IsZero() {
			return false
		}
	}
	return true
}

func snapshotHashesMatchPreparedEvent(snapshot Snapshot, config Configuration, prepared audit.Event) bool {
	expectedPaths, expectedHashes := auditSnapshot(snapshot.Entries, config)
	if !sameOrderedStrings(expectedPaths, prepared.Paths) || !sameOrderedStrings(expectedHashes, prepared.Hashes) {
		return false
	}

	declaredPaths := workflowSnapshotPaths(config)
	for _, entry := range snapshot.Entries {
		if entry.SHA256 != "" && !matchesDeclaredSnapshotPath(declaredPaths, entry.Path) {
			return false
		}
	}
	return true
}

func sameOrderedStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validSnapshotPathList(paths []string) bool {
	seen := make(map[string]struct{}, len(paths))
	for _, value := range paths {
		if !validSnapshotPattern(value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func samePathSet(left, right []string) bool {
	return strings.Join(uniqueSorted(left), "\x00") == strings.Join(uniqueSorted(right), "\x00")
}

func readAuditLog(root string) ([]auditLogRecord, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, errors.New("could not resolve audit workspace")
	}
	workspaceRoot, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return nil, errors.New("could not open audit workspace")
	}
	defer workspaceRoot.Close()

	directoryInfo, err := workspaceRoot.Lstat(".station")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil || directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return nil, errors.New("audit directory is not a regular directory")
	}
	directoryRoot, err := workspaceRoot.OpenRoot(".station")
	if err != nil {
		return nil, errors.New("could not open audit directory")
	}
	defer directoryRoot.Close()
	openedDirectoryInfo, err := directoryRoot.Stat(".")
	if err != nil || !os.SameFile(directoryInfo, openedDirectoryInfo) {
		return nil, errors.New("audit directory changed while opening")
	}

	logInfo, err := directoryRoot.Lstat("audit.jsonl")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil || logInfo.Mode()&os.ModeSymlink != 0 || !logInfo.Mode().IsRegular() {
		return nil, errors.New("audit log is not a regular file")
	}
	file, err := directoryRoot.Open("audit.jsonl")
	if err != nil {
		return nil, errors.New("could not open audit log")
	}
	defer file.Close()
	openedLogInfo, err := file.Stat()
	if err != nil || !os.SameFile(logInfo, openedLogInfo) {
		return nil, errors.New("audit log changed while opening")
	}
	if openedLogInfo.Size() == 0 {
		return nil, nil
	}
	var lastByte [1]byte
	if _, err := file.ReadAt(lastByte[:], openedLogInfo.Size()-1); err != nil || lastByte[0] != '\n' {
		return nil, errors.New("audit log has a truncated final record")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, errors.New("could not read audit log")
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), maxAuditRecordBytes)
	var records []auditLogRecord
	seenIDs := make(map[string]struct{})
	for scanner.Scan() {
		line := bytes.TrimSuffix(scanner.Bytes(), []byte("\r"))
		if len(line) == 0 {
			return nil, errors.New("audit log contains an empty record")
		}
		var record auditLogRecord
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return nil, errors.New("audit log contains a malformed record")
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, errors.New("audit log contains trailing record data")
		}
		if !validAuditLogRecord(record) {
			return nil, errors.New("audit log contains an invalid record")
		}
		if _, duplicate := seenIDs[record.EventID]; duplicate {
			return nil, errors.New("audit log contains duplicate event IDs")
		}
		seenIDs[record.EventID] = struct{}{}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("audit log contains an unreadable or oversized record")
	}
	return records, nil
}

func validAuditLogRecord(record auditLogRecord) bool {
	if !canonicalUUID(record.EventID) {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, record.Timestamp); err != nil {
		return false
	}
	switch record.Event.Kind {
	case audit.KindDecision, audit.KindCommand, audit.KindPrepared, audit.KindCompleted:
	default:
		return false
	}
	switch record.Event.Decision {
	case audit.DecisionAllow, audit.DecisionAsk, audit.DecisionDeny:
	default:
		return false
	}
	if record.Event.RunID != "" && !canonicalUUID(record.Event.RunID) {
		return false
	}
	if record.Event.Workflow != "" && !validAuditIdentifier(record.Event.Workflow) {
		return false
	}
	if record.Event.CommandID != "" && !validAuditIdentifier(record.Event.CommandID) || record.Event.Kind == audit.KindCommand && record.Event.CommandID == "" {
		return false
	}
	if record.Event.RelatedEventID != "" && !canonicalUUID(record.Event.RelatedEventID) {
		return false
	}
	if (record.Event.Kind == audit.KindPrepared || record.Event.Kind == audit.KindCompleted) && (record.Event.RunID == "" || record.Event.Workflow == "") {
		return false
	}
	for _, eventPath := range record.Event.Paths {
		if !safeAuditRecordPath(eventPath) {
			return false
		}
	}
	for _, category := range record.Event.Categories {
		switch category {
		case "source", "design", "test-output", "secret", "unknown":
		default:
			return false
		}
	}
	for _, digest := range record.Event.Hashes {
		if !validSHA256(digest) {
			return false
		}
	}
	return true
}

func validAuditIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || (index > 0 && (char == '-' || char == '_' || char == '.')) {
			continue
		}
		return false
	}
	return true
}

func safeAuditRecordPath(value string) bool {
	if value == "" || value == "." || len(value) > 4096 || !filepath.IsLocal(value) || !fs.ValidPath(filepath.ToSlash(value)) || strings.ContainsAny(value, "\\\r\n\x00=") {
		return false
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "-----begin") || strings.Contains(lower, "://") || strings.HasPrefix(strings.TrimSpace(lower), "bearer ") || auditPathLooksLikeCredentialHeader(lower) {
		return false
	}
	if strings.Contains(value, "/") || filepath.Ext(value) != "" {
		return true
	}
	tokens := auditPathTokens(lower)
	for index := 0; index+1 < len(tokens); index++ {
		first, second := tokens[index], tokens[index+1]
		switch {
		case first == "secret" && second == "value",
			first == "password" && second == "value",
			first == "passwd" && second == "value",
			first == "token" && second == "value",
			(first == "credential" || first == "credentials") && second == "value",
			first == "private" && second == "key",
			(first == "api" || first == "access") && second == "key",
			(first == "access" || first == "refresh" || first == "bearer") && second == "token":
			return false
		}
	}
	for index := 0; index+2 < len(tokens); index++ {
		if tokens[index] == "secret" && tokens[index+1] == "access" && tokens[index+2] == "key" {
			return false
		}
	}
	return true
}

func auditPathLooksLikeCredentialHeader(value string) bool {
	separator := strings.IndexByte(value, ':')
	if separator <= 0 || separator == len(value)-1 || strings.TrimSpace(value[separator+1:]) == "" {
		return false
	}
	for _, token := range auditPathTokens(value[:separator]) {
		switch token {
		case "authorization", "password", "passwd", "secret", "token", "credential", "credentials":
			return true
		}
	}
	return false
}

func auditPathTokens(value string) []string {
	return strings.FieldsFunc(value, func(char rune) bool {
		return !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9'))
	})
}

func findRunAuditEvents(records []auditLogRecord, state runState, workflowID, runID string) (*auditLogRecord, bool, error) {
	var prepared *auditLogRecord
	completed := false
	for index := range records {
		record := &records[index]
		if record.Event.Kind == audit.KindCompleted && record.Event.RunID == runID {
			completed = true
		}
		if record.Event.Kind == audit.KindPrepared && record.Event.RunID == runID {
			if record.EventID != state.PreparedEventID || record.Event.Workflow != workflowID || record.Event.Decision != audit.DecisionAllow || prepared != nil {
				return nil, false, workflowFailure(FailureDeny, ".station/audit.jsonl", "prepared workflow audit metadata is inconsistent")
			}
			prepared = record
		}
		if record.EventID == state.PreparedEventID {
			if record.Event.Kind != audit.KindPrepared || record.Event.RunID != runID || record.Event.Workflow != workflowID || record.Event.Decision != audit.DecisionAllow {
				return nil, false, workflowFailure(FailureDeny, ".station/audit.jsonl", "prepared workflow audit metadata is inconsistent")
			}
		}
	}
	return prepared, completed, nil
}

func verifyDeclaredOutputs(root string, config Configuration, manifest *workspace.Workspace, workflowID string, baseline, current Snapshot) ([]ContextArtifact, error) {
	baselineEntries := snapshotEntryMap(baseline.Entries)
	verified := make(map[string]ContextArtifact)
	for _, declaredPath := range config.Outputs {
		if !validSnapshotPattern(declaredPath) {
			return nil, workflowFailure(FailureInvalid, declaredPath, "workflow output path is invalid")
		}
		matches := findSnapshotMatches(declaredPath, current)
		if len(matches) == 0 {
			return nil, workflowFailure(FailureArtifactMissing, declaredPath, "declared workflow output is missing")
		}
		for _, entry := range matches {
			if !entry.Exists {
				return nil, workflowFailure(FailureArtifactMissing, entry.Path, "declared workflow output is missing")
			}
			if !validSHA256(entry.SHA256) {
				return nil, workflowFailure(FailureDeny, entry.Path, "declared workflow output has no safe content hash")
			}
			decision := policy.Authorize(policy.Action{Kind: policy.ActionWrite, ProjectID: projectForPath(manifest, entry.Path), Path: entry.Path}, manifest, workflowID)
			if decision.Outcome != policy.Allow {
				return nil, authorizationFailure(entry.Path, decision.Outcome)
			}
			if _, _, err := policy.AssertWorkspacePath(root, entry.Path, policy.PathWrite); err != nil {
				return nil, workflowFailure(FailureDeny, entry.Path, "declared workflow output is outside the workspace")
			}
			before := baselineEntries[entry.Path]
			artifact := ContextArtifact{
				Path:       entry.Path,
				Exists:     true,
				SHA256:     entry.SHA256,
				Bytes:      entry.Bytes,
				ModifiedAt: entry.ModifiedAt,
				Status:     "complete",
				HashAfter:  entry.SHA256,
			}
			if before.Exists {
				artifact.HashBefore = before.SHA256
			}
			verified[entry.Path] = artifact
		}
	}
	if len(verified) == 0 {
		return nil, workflowFailure(FailureArtifactMissing, "", "no declared workflow outputs were found")
	}
	outputs := make([]ContextArtifact, 0, len(verified))
	for _, output := range verified {
		outputs = append(outputs, output)
	}
	sort.Slice(outputs, func(i, j int) bool { return outputs[i].Path < outputs[j].Path })
	return outputs, nil
}

func changedPathsOutsideWriteScope(before, after Snapshot, writePaths []string) []string {
	beforeEntries := snapshotEntryMap(before.Entries)
	afterEntries := snapshotEntryMap(after.Entries)
	beforeExtra := snapshotExtraPathSet(before)
	afterExtra := snapshotExtraPathSet(after)
	allPaths := make(map[string]struct{}, len(beforeEntries)+len(afterEntries)+len(beforeExtra)+len(afterExtra))
	for value := range beforeEntries {
		allPaths[value] = struct{}{}
	}
	for value := range afterEntries {
		allPaths[value] = struct{}{}
	}
	for value := range beforeExtra {
		allPaths[value] = struct{}{}
	}
	for value := range afterExtra {
		allPaths[value] = struct{}{}
	}
	var changed []string
	for value := range allPaths {
		if matchesAnySnapshotPattern(writePaths, value) {
			continue
		}
		beforeEntry, beforeHasEntry := beforeEntries[value]
		afterEntry, afterHasEntry := afterEntries[value]
		if beforeHasEntry && afterHasEntry && entriesEqual(beforeEntry, afterEntry) {
			continue
		}
		if !beforeHasEntry && !afterHasEntry {
			_, beforeObserved := beforeExtra[value]
			_, afterObserved := afterExtra[value]
			if beforeObserved == afterObserved {
				continue
			}
		}
		changed = append(changed, value)
	}
	sort.Strings(changed)
	return changed
}

func snapshotExtraPathSet(snapshot Snapshot) map[string]struct{} {
	paths := make(map[string]struct{}, len(snapshot.ChangedPaths)+len(snapshot.OmittedPaths))
	for _, value := range snapshot.ChangedPaths {
		paths[value] = struct{}{}
	}
	for _, value := range snapshot.OmittedPaths {
		paths[value] = struct{}{}
	}
	return paths
}

func entriesEqual(left, right SnapshotEntry) bool {
	return left.Path == right.Path && left.Exists == right.Exists && left.SHA256 == right.SHA256 && left.Bytes == right.Bytes && left.ModifiedAt.Equal(right.ModifiedAt)
}

func findSnapshotMatches(pattern string, snapshot Snapshot) []SnapshotEntry {
	entries := snapshotEntryMap(snapshot.Entries)
	matches := matchingSnapshotEntries(pattern, entries)
	if !hasSnapshotWildcard(pattern) && len(matches) == 0 {
		return []SnapshotEntry{{Path: pattern}}
	}
	return matches
}
