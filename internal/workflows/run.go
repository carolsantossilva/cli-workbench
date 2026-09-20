package workflows

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"station/internal/audit"
	"station/internal/policy"
	"station/internal/workspace"
)

type FailureCode string

const (
	FailureInvalid             FailureCode = "invalid"
	FailureAsk                 FailureCode = "ask"
	FailureDeny                FailureCode = "deny"
	FailureArtifactMissing     FailureCode = "artifact-missing"
	FailurePrerequisiteMissing FailureCode = "prerequisite-missing"
)

type WorkflowError struct {
	Code    FailureCode
	Path    string
	Message string
}

func (err *WorkflowError) Error() string { return err.Message }

func prepareWorkflow(root, id string, input Input) (ContextPacket, string, error) {
	if root == "" {
		return ContextPacket{}, "", workflowFailure(FailureInvalid, "", "workspace root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailureInvalid, "", "could not resolve workspace root")
	}
	manifest, err := workflowManifest(absoluteRoot, input.Manifest)
	if err != nil {
		return ContextPacket{}, "", err
	}
	resolved, err := Resolve(id, manifest)
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailureInvalid, "/workflows/"+id, err.Error())
	}
	if err := validateWorkflowManifest(manifest, absoluteRoot); err != nil {
		return ContextPacket{}, "", err
	}
	if len(resolved.Configuration.Outputs) == 0 {
		return ContextPacket{}, "", workflowFailure(FailureInvalid, "/workflows/"+id+"/outputs", "workflow outputs must be declared")
	}
	dataPolicy, err := policy.ParseDataPolicy(manifest.Policies.Data)
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailureInvalid, "/policies/data", "data policy is invalid")
	}
	manifestSHA256, err := manifestFingerprint(manifest)
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailureInvalid, "workspace.json", "workspace manifest is invalid")
	}

	rootHandle, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailureInvalid, "", "could not open workspace root")
	}
	defer rootHandle.Close()

	if err := authorizeInstruction(absoluteRoot, rootHandle, id, resolved, manifest); err != nil {
		return ContextPacket{}, "", err
	}
	instruction, err := readSmallTextFile(absoluteRoot, rootHandle, resolved.Definition.InstructionPath, policy.PathRead)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ContextPacket{}, "", workflowFailure(FailurePrerequisiteMissing, resolved.Definition.InstructionPath, "versioned workflow instruction is missing")
		}
		return ContextPacket{}, "", workflowFailure(FailureDeny, resolved.Definition.InstructionPath, "versioned workflow instruction is not safely readable")
	}
	instructionPolicy := dataPolicy
	instructionPolicy.ProjectID = projectForPath(manifest, resolved.Definition.InstructionPath)
	instructionScan := policy.ScanData(resolved.Definition.InstructionPath, instruction, dataCategoryForPath(manifest, resolved.Definition.InstructionPath), instructionPolicy)
	if instructionScan.Decision.Outcome != policy.Allow {
		return ContextPacket{}, "", authorizationFailure(resolved.Definition.InstructionPath, instructionScan.Decision.Outcome)
	}

	declaredPaths := workflowSnapshotPaths(resolved.Configuration)
	declaredSnapshot, err := snapshotWorkspace(absoluteRoot, declaredPaths)
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailureDeny, "", "could not safely snapshot declared workflow paths")
	}
	entryByPath := snapshotEntryMap(declaredSnapshot.Entries)
	inputs, omissions, categories, err := prepareInputs(absoluteRoot, rootHandle, resolved, manifest, dataPolicy, entryByPath, declaredSnapshot.OmittedPaths)
	if err != nil {
		return ContextPacket{}, "", err
	}
	outputs, err := expectedOutputStates(absoluteRoot, resolved, manifest, entryByPath)
	if err != nil {
		return ContextPacket{}, "", err
	}
	summaries := summarizeConfiguredPaths(resolved.Configuration.Summaries, entryByPath)

	runID, err := newRunID()
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailurePrerequisiteMissing, "", "could not generate a workflow run ID")
	}
	runSnapshot := declaredSnapshot
	if declaredSnapshot.Source == "filesystem" {
		metadataSnapshot, snapshotErr := snapshotFilesystemMetadata(absoluteRoot)
		if snapshotErr != nil {
			return ContextPacket{}, "", workflowFailure(FailureDeny, "", "could not safely capture workspace metadata")
		}
		runSnapshot = mergeDeclaredSnapshot(metadataSnapshot, declaredSnapshot, declaredPaths)
	}
	auditPaths, auditHashes := auditSnapshot(declaredSnapshot.Entries, resolved.Configuration)
	preparedEventID, _, err := audit.Append(absoluteRoot, audit.Event{
		Kind:       audit.KindPrepared,
		Decision:   audit.DecisionAllow,
		RunID:      runID,
		Workflow:   id,
		Paths:      auditPaths,
		Categories: categories,
		Hashes:     auditHashes,
	})
	if err != nil {
		return ContextPacket{}, "", workflowFailure(FailurePrerequisiteMissing, ".station/audit.jsonl", "could not append prepared workflow audit event")
	}
	state := runState{
		Version:         1,
		RunID:           runID,
		WorkflowID:      id,
		PreparedEventID: preparedEventID,
		ManifestSHA256:  manifestSHA256,
		Snapshot:        runSnapshot,
		OutputPaths:     append([]string(nil), resolved.Configuration.Outputs...),
		CreatedAt:       time.Now().UTC(),
	}
	if err := writeRunState(absoluteRoot, state); err != nil {
		return ContextPacket{}, "", workflowFailure(FailurePrerequisiteMissing, ".station/runs", "could not persist the prepared workflow snapshot")
	}

	nextHandoff := resolved.Configuration.Handoff
	if nextHandoff == "" {
		nextHandoff = resolved.Definition.Handoff
	}
	packet := ContextPacket{
		WorkflowID:      id,
		WorkspaceRoot:   absoluteRoot,
		InstructionPath: resolved.Definition.InstructionPath,
		Instruction:     instruction,
		Inputs:          inputs,
		Summaries:       summaries,
		AllowedSkills:   append([]string(nil), resolved.Configuration.AllowedSkills...),
		AllowedCommands: append([]string(nil), resolved.Configuration.AllowedCommands...),
		ReadPaths:       append([]string(nil), resolved.Configuration.ReadPaths...),
		WritePaths:      append([]string(nil), resolved.Configuration.WritePaths...),
		ExpectedOutputs: outputs,
		NextHandoff:     nextHandoff,
		Omissions:       omissions,
		Warnings: []string{
			"Station does not control host tools; observed changes do not prove prevention of external writes.",
			"Out-of-scope filesystem metadata uses path, existence, size, and modification time only; edits preserving both size and modification time may be missed without Git evidence. Git path-name evidence may not distinguish edits to paths already changed at preparation. Secret, environment, binary, symlink, and cache contents are not persisted in snapshots.",
		},
	}
	return packet, runID, nil
}

func PrepareWorkflow(root, id string, input Input) (ContextPacket, string, error) {
	return prepareWorkflow(root, id, input)
}

func workflowManifest(root string, supplied *workspace.Workspace) (*workspace.Workspace, error) {
	if supplied != nil {
		if supplied.SchemaVersion != 1 {
			return nil, workflowFailure(FailureInvalid, "/schemaVersion", "unsupported workspace schema version")
		}
		return supplied, nil
	}
	if err := validateWorkspaceManifestFile(root); err != nil {
		return nil, workflowFailure(FailureInvalid, "/workspace.json", "workspace manifest must be a regular non-symlink file")
	}
	manifest, err := workspace.LoadWorkspace(root)
	if err != nil {
		return nil, workflowFailure(FailureInvalid, "/workspace.json", "workspace manifest is invalid or missing")
	}
	return manifest, nil
}

func validateWorkflowManifest(manifest *workspace.Workspace, root string) error {
	diagnostics := workspace.ValidateWorkspace(manifest, root)
	diagnostics = append(diagnostics, policy.ValidateManifestPolicies(manifest)...)
	if len(diagnostics) > 0 {
		first := diagnostics[0]
		return workflowFailure(FailureInvalid, first.Path, "workspace validation failed: "+first.Message)
	}
	return nil
}

func authorizeInstruction(root string, rootHandle *os.Root, workflowID string, resolved Resolved, manifest *workspace.Workspace) error {
	path := resolved.Definition.InstructionPath
	symlink, err := hasSymlinkComponent(rootHandle, path)
	if err != nil || symlink {
		return workflowFailure(FailureDeny, path, "workflow instruction must not traverse a symlink")
	}
	info, err := rootHandle.Lstat(path)
	if err != nil {
		return workflowFailure(FailurePrerequisiteMissing, path, "versioned workflow instruction is missing")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return workflowFailure(FailureDeny, path, "workflow instruction must be a regular non-symlink file")
	}
	decision := policy.Authorize(policy.Action{Kind: policy.ActionRead, Path: path}, manifest, workflowID)
	if decision.Outcome != policy.Allow {
		return authorizationFailure(path, decision.Outcome)
	}
	if _, _, err := policy.AssertWorkspacePath(root, path, policy.PathRead); err != nil {
		return workflowFailure(FailureDeny, path, "workflow instruction is outside the workspace")
	}
	return nil
}

func readSmallTextFile(root string, rootHandle *os.Root, path string, mode policy.PathMode) (string, error) {
	symlink, err := hasSymlinkComponent(rootHandle, path)
	if err != nil {
		return "", err
	}
	if symlink {
		return "", errContextContentOmitted
	}
	info, err := rootHandle.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errContextContentOmitted
	}
	if _, _, err := policy.AssertWorkspacePath(root, path, mode); err != nil {
		return "", err
	}
	file, err := rootHandle.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		return "", fmt.Errorf("file changed while opening")
	}
	info = openedInfo
	if !info.Mode().IsRegular() || info.Size() > maxContextInputBytes {
		return "", errContextContentOmitted
	}
	content, err := io.ReadAll(io.LimitReader(file, maxContextInputBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(content)) > maxContextInputBytes || !isTextContent(content) {
		return "", errContextContentOmitted
	}
	return string(content), nil
}

func prepareInputs(root string, rootHandle *os.Root, resolved Resolved, manifest *workspace.Workspace, dataPolicy policy.DataPolicy, entries map[string]SnapshotEntry, omittedPaths []string) ([]ContextInput, []ContextOmission, []string, error) {
	var inputs []ContextInput
	var omissions []ContextOmission
	categorySet := make(map[string]struct{})
	for _, declaredPath := range resolved.Configuration.Inputs {
		matched := matchingSnapshotEntries(declaredPath, entries)
		omittedMatches := matchingOmittedPaths(declaredPath, omittedPaths)
		for _, omittedPath := range omittedMatches {
			reason := "secret, binary, or symlinked content is excluded"
			if isEnvironmentPath(omittedPath) {
				reason = "secret files are excluded"
			}
			omissions = append(omissions, ContextOmission{Path: omittedPath, Reason: reason})
		}
		if len(matched) == 0 {
			if isEnvironmentPath(declaredPath) {
				if len(omittedMatches) == 0 {
					omissions = append(omissions, ContextOmission{Path: declaredPath, Reason: "secret files are excluded"})
				}
				continue
			}
			if len(omittedMatches) > 0 {
				continue
			}
			return nil, nil, nil, workflowFailure(FailureArtifactMissing, declaredPath, "required workflow input is missing")
		}
		for _, entry := range matched {
			if !entry.Exists {
				return nil, nil, nil, workflowFailure(FailureArtifactMissing, entry.Path, "required workflow input is missing")
			}
			if isEnvironmentPath(entry.Path) {
				omissions = append(omissions, ContextOmission{Path: entry.Path, Reason: "secret files are excluded"})
				continue
			}
			if entry.SHA256 == "" {
				omissions = append(omissions, ContextOmission{Path: entry.Path, Reason: "secret, binary, or symlinked content is excluded"})
				continue
			}
			decision := policy.Authorize(policy.Action{Kind: policy.ActionRead, ProjectID: projectForPath(manifest, entry.Path), Path: entry.Path}, manifest, resolved.Definition.ID)
			if decision.Outcome != policy.Allow {
				return nil, nil, nil, authorizationFailure(entry.Path, decision.Outcome)
			}
			if entry.Bytes > maxContextInputBytes {
				omissions = append(omissions, ContextOmission{Path: entry.Path, Reason: "input exceeds the context size limit"})
				continue
			}
			content, err := readSmallTextFile(root, rootHandle, entry.Path, policy.PathRead)
			if err != nil {
				if errors.Is(err, errContextContentOmitted) {
					omissions = append(omissions, ContextOmission{Path: entry.Path, Reason: "binary or non-text inputs are excluded"})
					continue
				}
				return nil, nil, nil, workflowFailure(FailureDeny, entry.Path, "workflow input is not safely readable")
			}
			category := dataCategoryForPath(manifest, entry.Path)
			pathPolicy := dataPolicy
			pathPolicy.ProjectID = projectForPath(manifest, entry.Path)
			result := policy.ScanData(entry.Path, content, category, pathPolicy)
			if result.Decision.Outcome != policy.Allow {
				omissions = append(omissions, ContextOmission{Path: entry.Path, Reason: "data policy omitted this input"})
				continue
			}
			inputs = append(inputs, ContextInput{
				Path:      entry.Path,
				SHA256:    entry.SHA256,
				Bytes:     entry.Bytes,
				LineCount: fileLineCount([]byte(content)),
				Role:      "input",
				Content:   content,
			})
			categorySet[category] = struct{}{}
		}
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Path < inputs[j].Path })
	sort.Slice(omissions, func(i, j int) bool { return omissions[i].Path < omissions[j].Path })
	categories := make([]string, 0, len(categorySet))
	for category := range categorySet {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return inputs, omissions, categories, nil
}

func expectedOutputStates(root string, resolved Resolved, manifest *workspace.Workspace, entries map[string]SnapshotEntry) ([]ContextArtifact, error) {
	var outputs []ContextArtifact
	for _, declaredPath := range resolved.Configuration.Outputs {
		if !validSnapshotPattern(declaredPath) {
			return nil, workflowFailure(FailureInvalid, "/workflows/"+resolved.Definition.ID+"/outputs", "workflow output path is invalid")
		}
		matched := matchingSnapshotEntries(declaredPath, entries)
		if !hasSnapshotWildcard(declaredPath) && len(matched) == 0 {
			matched = []SnapshotEntry{{Path: declaredPath}}
		}
		if hasSnapshotWildcard(declaredPath) && len(matched) == 0 {
			matched = []SnapshotEntry{{Path: declaredPath}}
		}
		for _, entry := range matched {
			decision := policy.Authorize(policy.Action{Kind: policy.ActionWrite, ProjectID: projectForPath(manifest, entry.Path), Path: entry.Path}, manifest, resolved.Definition.ID)
			if decision.Outcome != policy.Allow {
				return nil, authorizationFailure(entry.Path, decision.Outcome)
			}
			if _, _, err := policy.AssertWorkspacePath(root, entry.Path, policy.PathWrite); err != nil {
				return nil, workflowFailure(FailureDeny, entry.Path, "workflow output is outside the workspace")
			}
			status := "pending"
			if entry.Exists {
				status = "present"
			}
			outputs = append(outputs, ContextArtifact{
				Path:       entry.Path,
				Exists:     entry.Exists,
				SHA256:     entry.SHA256,
				Bytes:      entry.Bytes,
				ModifiedAt: entry.ModifiedAt,
				Status:     status,
				HashBefore: entry.SHA256,
			})
		}
	}
	sort.Slice(outputs, func(i, j int) bool { return outputs[i].Path < outputs[j].Path })
	return outputs, nil
}

func summarizeConfiguredPaths(paths []string, entries map[string]SnapshotEntry) []ContextSummary {
	var summaries []ContextSummary
	for _, declaredPath := range paths {
		matched := matchingSnapshotEntries(declaredPath, entries)
		if len(matched) == 0 {
			summaries = append(summaries, ContextSummary{Path: declaredPath, SummaryKind: "metadata", Content: "not present in the local snapshot"})
			continue
		}
		for _, entry := range matched {
			content := "missing"
			if entry.Exists {
				content = fmt.Sprintf("sha256=%s bytes=%d mtime=%s", entry.SHA256, entry.Bytes, entry.ModifiedAt.Format(time.RFC3339Nano))
			}
			summaries = append(summaries, ContextSummary{Path: entry.Path, SummaryKind: "metadata", Content: content})
		}
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Path < summaries[j].Path })
	return summaries
}

func workflowSnapshotPaths(config Configuration) []string {
	paths := append([]string(nil), config.Inputs...)
	paths = append(paths, config.Outputs...)
	paths = append(paths, config.ReadPaths...)
	paths = append(paths, config.WritePaths...)
	paths = append(paths, config.Summaries...)
	return uniqueSorted(paths)
}

func auditSnapshot(entries []SnapshotEntry, config Configuration) ([]string, []string) {
	declared := workflowSnapshotPaths(config)
	type pathHash struct {
		path string
		hash string
	}
	values := make([]pathHash, 0, len(entries))
	for _, entry := range entries {
		if !entry.Exists || entry.SHA256 == "" || !matchesDeclaredSnapshotPath(declared, entry.Path) {
			continue
		}
		values = append(values, pathHash{path: entry.Path, hash: entry.SHA256})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].path < values[j].path })
	paths := make([]string, 0, len(values))
	hashes := make([]string, 0, len(values))
	for _, value := range values {
		paths = append(paths, value.path)
		hashes = append(hashes, value.hash)
	}
	return paths, hashes
}

func snapshotEntryMap(entries []SnapshotEntry) map[string]SnapshotEntry {
	result := make(map[string]SnapshotEntry, len(entries))
	for _, entry := range entries {
		result[entry.Path] = entry
	}
	return result
}

func matchingSnapshotEntries(pattern string, entries map[string]SnapshotEntry) []SnapshotEntry {
	var matched []SnapshotEntry
	for path, entry := range entries {
		if path == pattern || hasSnapshotWildcard(pattern) && matchesSnapshotPattern(pattern, path) {
			matched = append(matched, entry)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Path < matched[j].Path })
	return matched
}

func matchingOmittedPaths(pattern string, omittedPaths []string) []string {
	var matched []string
	for _, omittedPath := range omittedPaths {
		if omittedPath == pattern || hasSnapshotWildcard(pattern) && matchesSnapshotPattern(pattern, omittedPath) {
			matched = append(matched, omittedPath)
		}
	}
	sort.Strings(matched)
	return matched
}

func matchesAnySnapshotPattern(patterns []string, candidate string) bool {
	for _, pattern := range patterns {
		if pattern == candidate || hasSnapshotWildcard(pattern) && matchesSnapshotPattern(pattern, candidate) {
			return true
		}
	}
	return false
}

func dataCategoryForPath(manifest *workspace.Workspace, value string) string {
	if isEnvironmentPath(value) {
		return "secret"
	}
	if value == "docs/station" || strings.HasPrefix(value, "docs/station/") {
		return "design"
	}
	for _, project := range manifest.Projects {
		if value == project.Path || strings.HasPrefix(value, strings.TrimSuffix(project.Path, "/")+"/") {
			return "source"
		}
	}
	return "unknown"
}

func isEnvironmentPath(value string) bool {
	return strings.HasPrefix(path.Base(value), ".env")
}

func projectForPath(manifest *workspace.Workspace, value string) string {
	for _, project := range manifest.Projects {
		if value == project.Path || strings.HasPrefix(value, strings.TrimSuffix(project.Path, "/")+"/") {
			return project.ID
		}
	}
	return ""
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func authorizationFailure(path string, outcome policy.Outcome) error {
	switch outcome {
	case policy.Ask:
		return workflowFailure(FailureAsk, path, "workflow action requires review")
	case policy.Deny:
		return workflowFailure(FailureDeny, path, "workflow action is denied by policy")
	default:
		return workflowFailure(FailureDeny, path, "workflow action is not explicitly allowed")
	}
}

func workflowFailure(code FailureCode, path, message string) error {
	return &WorkflowError{Code: code, Path: path, Message: message}
}

func newRunID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func validRunID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}
