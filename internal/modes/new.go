package modes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"station/internal/scaffold"
)

type NewOptions struct {
	Design scaffold.Design
	Policy scaffold.Policy
}

type NewResult struct {
	Root        string   `json:"root"`
	WorkspaceID string   `json:"workspaceId"`
	Files       []string `json:"files"`
}

func createNewWorkspace(root, idea string, options NewOptions) (NewResult, error) {
	if strings.TrimSpace(idea) == "" || !utf8.ValidString(idea) {
		return NewResult{}, errors.New("idea must contain valid, non-whitespace text")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return NewResult{}, fmt.Errorf("resolve workspace destination: %w", err)
	}
	policies := options.Policy
	if len(policies.Execution) == 0 && len(policies.Data) == 0 && len(policies.ExternalChanges) == 0 {
		policies = scaffold.DefaultPolicy()
	}
	created, err := scaffold.ScaffoldMonorepo(absoluteRoot, options.Design, policies)
	if err != nil {
		return NewResult{}, err
	}
	rootHandle, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return NewResult{}, fmt.Errorf("open generated workspace: %w", err)
	}
	defer rootHandle.Close()
	content := []byte(idea)
	if len(content) == 0 || content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	file, err := rootHandle.OpenFile("docs/station/idea.md", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return NewResult{}, fmt.Errorf("write idea input: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return NewResult{}, fmt.Errorf("write idea input: %w", err)
	}
	if err := file.Close(); err != nil {
		return NewResult{}, fmt.Errorf("close idea input: %w", err)
	}
	files := append(created.Files, "docs/station/idea.md")
	sort.Strings(files)
	return NewResult{Root: absoluteRoot, WorkspaceID: created.WorkspaceID, Files: files}, nil
}

func CreateNewWorkspace(root, idea string, options NewOptions) (NewResult, error) {
	return createNewWorkspace(root, idea, options)
}
