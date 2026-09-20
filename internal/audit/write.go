// Package audit appends redacted Station event metadata to a workspace-local log.
package audit

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const auditDirectory = ".station"
const auditFilename = "audit.jsonl"

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

type Kind string

const (
	KindDecision  Kind = "decision"
	KindCommand   Kind = "command"
	KindPrepared  Kind = "prepared"
	KindCompleted Kind = "completed"
)

// Event contains only redacted metadata. Paths must be relative, pre-redacted
// paths; content, secret values, and arbitrary detail maps are intentionally
// not part of this API. The writer rejects recognizable embedded value and
// credential markers, but callers remain responsible for redacting path-shaped
// data that cannot be distinguished from a legitimate filename.
type Event struct {
	Kind           Kind     `json:"kind"`
	Decision       Decision `json:"decision"`
	RunID          string   `json:"runId,omitempty"`
	Workflow       string   `json:"workflow,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	Categories     []string `json:"categories,omitempty"`
	Hashes         []string `json:"hashes,omitempty"`
	RelatedEventID string   `json:"relatedEventId,omitempty"`
}

type record struct {
	EventID   string `json:"eventId"`
	Timestamp string `json:"timestamp"`
	Event
}

// Append writes one validated event as a JSONL record beneath root and returns
// its generated ID and absolute log path. Existing records are never rewritten.
func Append(root string, event Event) (eventID, path string, err error) {
	return appendAudit(root, event)
}

func appendAudit(root string, event Event) (eventID, path string, err error) {
	if root == "" {
		return "", "", errors.New("audit root is required")
	}
	if err := validateEvent(event); err != nil {
		return "", "", err
	}

	eventID, err = newEventID()
	if err != nil {
		return "", "", fmt.Errorf("generate audit event ID: %w", err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve audit root: %w", err)
	}
	workspaceRoot, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return "", "", fmt.Errorf("open audit root: %w", err)
	}
	defer workspaceRoot.Close()

	if err := workspaceRoot.Mkdir(auditDirectory, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", "", fmt.Errorf("create audit directory: %w", err)
	}
	directoryInfo, err := workspaceRoot.Lstat(auditDirectory)
	if err != nil {
		return "", "", fmt.Errorf("inspect audit directory: %w", err)
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("audit directory must be a directory, not a symlink")
	}
	auditRoot, err := workspaceRoot.OpenRoot(auditDirectory)
	if err != nil {
		return "", "", fmt.Errorf("open audit directory: %w", err)
	}
	defer auditRoot.Close()

	if info, err := auditRoot.Lstat(auditFilename); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("audit log must be a regular file, not a symlink or special file")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", fmt.Errorf("inspect audit log: %w", err)
	}

	file, err := auditRoot.OpenFile(auditFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return "", "", fmt.Errorf("open audit log: %w", err)
	}
	path = filepath.Join(absoluteRoot, auditDirectory, auditFilename)
	line, err := json.Marshal(record{EventID: eventID, Timestamp: timestamp, Event: event})
	if err != nil {
		file.Close()
		return "", "", fmt.Errorf("encode audit event: %w", err)
	}
	line = append(line, '\n')
	written, writeErr := file.Write(line)
	closeErr := file.Close()
	if writeErr != nil {
		return eventID, path, fmt.Errorf("append audit event: %w", writeErr)
	}
	if written != len(line) {
		return eventID, path, fmt.Errorf("append audit event: %w", io.ErrShortWrite)
	}
	if closeErr != nil {
		return eventID, path, fmt.Errorf("close audit log: %w", closeErr)
	}
	return eventID, path, nil
}

func validateEvent(event Event) error {
	switch event.Kind {
	case KindDecision, KindCommand, KindPrepared, KindCompleted:
	default:
		return fmt.Errorf("invalid audit event kind")
	}
	switch event.Decision {
	case DecisionAllow, DecisionAsk, DecisionDeny:
	default:
		return fmt.Errorf("invalid or undecided audit decision")
	}
	if event.RunID != "" && !validIdentifier(event.RunID) {
		return fmt.Errorf("invalid audit run ID")
	}
	if event.Workflow != "" && !validIdentifier(event.Workflow) {
		return fmt.Errorf("invalid audit workflow ID")
	}
	if event.RelatedEventID != "" && !validEventID(event.RelatedEventID) {
		return fmt.Errorf("invalid related audit event ID")
	}
	for _, path := range event.Paths {
		if path == "" || path == "." || len(path) > 4096 || !filepath.IsLocal(path) || !fs.ValidPath(filepath.ToSlash(path)) || unsafeAuditPathMetadata(path) {
			return fmt.Errorf("audit paths must be relative, local, and redacted")
		}
	}
	for _, category := range event.Categories {
		switch category {
		case "source", "design", "test-output", "secret", "unknown":
		default:
			return fmt.Errorf("invalid audit data category")
		}
	}
	for _, hash := range event.Hashes {
		if len(hash) != 64 {
			return fmt.Errorf("audit hashes must be SHA-256 hex digests")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return fmt.Errorf("audit hashes must be SHA-256 hex digests")
		}
	}
	return nil
}

func unsafeAuditPathMetadata(path string) bool {
	if strings.ContainsAny(path, "\\\r\n\x00=") || strings.Contains(path, "://") {
		return true
	}
	lower := strings.ToLower(path)
	if strings.Contains(lower, "-----begin") {
		return true
	}
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	for i := 0; i+1 < len(tokens); i++ {
		first, second := tokens[i], tokens[i+1]
		switch {
		case first == "secret" && second == "value",
			first == "password" && second == "value",
			first == "passwd" && second == "value",
			first == "token" && second == "value",
			(first == "credential" || first == "credentials") && second == "value",
			first == "private" && second == "key",
			(first == "api" || first == "access") && second == "key",
			(first == "access" || first == "refresh" || first == "bearer") && second == "token":
			return true
		}
	}
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i] == "secret" && tokens[i+1] == "access" && tokens[i+2] == "key" {
			return true
		}
	}
	return false
}

func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (i > 0 && (r == '-' || r == '_' || r == '.')) {
			continue
		}
		return false
	}
	return true
}

func validEventID(value string) bool {
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

func newEventID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
