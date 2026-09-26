package scaffold

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"station/internal/assets"
	"station/internal/policy"
	"station/internal/workflows"
	"station/internal/workspace"
)

type Design struct {
	Shared     bool
	ModulePath string
}

type Policy = workspace.Policies

type ScaffoldResult struct {
	WorkspaceID string   `json:"workspaceId"`
	Files       []string `json:"files"`
}

var ErrDestinationNotEmpty = errors.New("destination directory is not empty")

var workflowStages = []struct {
	id     string
	inputs []string
	output string
}{
	{id: "brainstorm", inputs: []string{"docs/station/idea.md"}, output: "docs/station/brief.md"},
	{id: "grill-me", inputs: []string{"docs/station/brief.md"}, output: "docs/station/decisions.md"},
	{id: "design", inputs: []string{"docs/station/brief.md", "docs/station/decisions.md"}, output: "docs/station/design.md"},
	{id: "build", inputs: []string{"docs/station/design.md"}, output: "docs/station/plan.md"},
	{id: "review", inputs: []string{"docs/station/decisions.md", "docs/station/design.md", "docs/station/plan.md"}, output: "docs/station/decisions.md"},
}

func DefaultPolicy() Policy {
	return Policy{
		Execution: map[string]any{"default": "ask"},
		Data: map[string]any{
			"default": "ask",
			"categories": map[string]any{
				"design":  "allow",
				"secret":  "deny",
				"unknown": "ask",
			},
		},
		ExternalChanges: map[string]any{"default": "deny"},
	}
}

// ScaffoldMonorepo writes only the embedded, non-executable workspace contract
// into an existing empty directory. It never installs dependencies or runs a
// generator, and it refuses to replace even one pre-existing entry.
func ScaffoldMonorepo(root string, design Design, policies Policy) (ScaffoldResult, error) {
	if root == "" {
		return ScaffoldResult{}, errors.New("workspace destination is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return ScaffoldResult{}, fmt.Errorf("resolve workspace destination: %w", err)
	}
	rootHandle, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return ScaffoldResult{}, fmt.Errorf("open workspace destination: %w", err)
	}
	defer rootHandle.Close()
	if err := requireEmptyDirectory(rootHandle); err != nil {
		return ScaffoldResult{}, err
	}
	if err := validateModulePath(design.ModulePath); err != nil {
		return ScaffoldResult{}, err
	}

	if emptyPolicy(policies) {
		policies = DefaultPolicy()
	}
	manifest := newManifest(absoluteRoot, design, policies)
	if diagnostics := workspace.ValidateWorkspace(&manifest, absoluteRoot); len(diagnostics) != 0 {
		return ScaffoldResult{}, fmt.Errorf("generated workspace contract is invalid: %s", diagnostics[0].Message)
	}
	if diagnostics := policy.ValidateManifestPolicies(&manifest); len(diagnostics) != 0 {
		return ScaffoldResult{}, fmt.Errorf("generated workspace policy is invalid: %s", diagnostics[0].Message)
	}
	encodedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ScaffoldResult{}, fmt.Errorf("encode workspace manifest: %w", err)
	}
	encodedManifest = append(encodedManifest, '\n')

	files, err := embeddedFiles()
	if err != nil {
		return ScaffoldResult{}, err
	}
	files["schemas/workspace.schema.json"] = assets.WorkspaceSchema()
	files["workspace.json"] = encodedManifest
	if design.Shared {
		files["packages/shared/README.md"] = []byte("# Shared contracts\n\nAdd platform-independent contracts here only when an approved design requires them.\n")
	}
	if design.ModulePath != "" {
		files["apps/server/go.mod"] = []byte("module " + design.ModulePath + "\n\ngo 1.26\n")
	}

	paths := make([]string, 0, len(files))
	for name := range files {
		if !fs.ValidPath(name) || filepath.IsAbs(name) {
			return ScaffoldResult{}, fmt.Errorf("embedded scaffold path is not workspace-relative: %q", name)
		}
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		if err := createFile(rootHandle, name, files[name]); err != nil {
			return ScaffoldResult{}, fmt.Errorf("write scaffold file %s: %w", name, err)
		}
	}
	return ScaffoldResult{WorkspaceID: manifest.Workspace.ID, Files: paths}, nil
}

func scaffoldMonorepo(root string, design Design, policies Policy) (ScaffoldResult, error) {
	return ScaffoldMonorepo(root, design, policies)
}

func newManifest(root string, design Design, policies Policy) workspace.Workspace {
	projects := []workspace.Project{
		{ID: "mobile", Path: "apps/mobile", Kind: "mobile", Language: "typescript"},
		{ID: "web", Path: "apps/web", Kind: "web", Language: "typescript"},
		{ID: "server", Path: "apps/server", Kind: "server", Language: "go"},
	}
	if design.Shared {
		projects = append(projects, workspace.Project{ID: "shared", Path: "packages/shared", Kind: "shared", Language: "typescript"})
	}
	manifest := workspace.Workspace{
		SchemaVersion:        1,
		Workspace:            workspace.Identity{ID: workspaceID(root), Kind: "monorepo"},
		Projects:             projects,
		Commands:             []workspace.Command{},
		Policies:             policies,
		Workflows:            make(map[string]workspace.Workflow, len(workflowStages)),
		ExternalRepositories: []workspace.ExternalRepository{},
	}
	for _, stage := range workflowStages {
		configuration := workspace.Workflow{
			RequiredSkills:  []string{},
			AllowedSkills:   []string{"station-mobile", "station-web", "station-server", "adr-authoring"},
			AllowedCommands: []string{},
			ReadPaths:       []string{"docs/station/**", ".agents/skills/adr-authoring/SKILL.md"},
			WritePaths:      []string{stage.output},
			Inputs:          append([]string(nil), stage.inputs...),
			Outputs:         []string{stage.output},
			Summaries:       []string{},
		}
		resolved, err := workflows.Resolve(stage.id, &workspace.Workspace{Workflows: map[string]workspace.Workflow{stage.id: configuration}})
		if err == nil {
			configuration.Handoff = resolved.Definition.Handoff
		}
		manifest.Workflows[stage.id] = configuration
	}
	return manifest
}

func workspaceID(root string) string {
	name := filepath.Base(filepath.Clean(root))
	if name == "." || name == string(filepath.Separator) || strings.TrimSpace(name) == "" {
		return "workspace"
	}
	return name
}

func embeddedFiles() (map[string][]byte, error) {
	assets, err := templates()
	if err != nil {
		return nil, fmt.Errorf("open embedded scaffold templates: %w", err)
	}
	files := make(map[string][]byte)
	err = fs.WalkDir(assets, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("template is not a regular file: %s", name)
		}
		content, err := fs.ReadFile(assets, name)
		if err != nil {
			return fmt.Errorf("read embedded template %s: %w", name, err)
		}
		files[name] = content
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk embedded scaffold templates: %w", err)
	}
	return files, nil
}

func requireEmptyDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open workspace destination: %w", err)
	}
	entries, readErr := directory.ReadDir(1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("inspect workspace destination: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close workspace destination: %w", closeErr)
	}
	if len(entries) != 0 {
		return ErrDestinationNotEmpty
	}
	return nil
}

func validateModulePath(modulePath string) error {
	if modulePath == "" {
		return nil
	}
	if strings.TrimSpace(modulePath) != modulePath || strings.ContainsAny(modulePath, "\\\r\n\t\x00 ") ||
		filepath.IsAbs(modulePath) || !fs.ValidPath(modulePath) {
		return fmt.Errorf("module path must be an explicit, relative Go module path")
	}
	for _, segment := range strings.Split(modulePath, "/") {
		if !validModulePathElement(segment) {
			return fmt.Errorf("module path must be an explicit, relative Go module path")
		}
	}
	return nil
}

func validModulePathElement(segment string) bool {
	if segment == "" || segment[0] == '.' || segment[len(segment)-1] == '.' {
		return false
	}
	for _, character := range segment {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("-._~", character) {
			continue
		}
		return false
	}

	prefix := strings.SplitN(segment, ".", 2)[0]
	upperPrefix := strings.ToUpper(prefix)
	switch upperPrefix {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(upperPrefix) == 4 && upperPrefix[3] >= '1' && upperPrefix[3] <= '9' &&
		(upperPrefix[:3] == "COM" || upperPrefix[:3] == "LPT") {
		return false
	}
	if tilde := strings.LastIndexByte(prefix, '~'); tilde >= 0 && tilde < len(prefix)-1 {
		suffix := prefix[tilde+1:]
		digitsOnly := true
		for _, character := range suffix {
			if character < '0' || character > '9' {
				digitsOnly = false
				break
			}
		}
		if digitsOnly {
			return false
		}
	}
	return true
}

func createFile(root *os.Root, name string, content []byte) error {
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(name)))
	if parent != "." {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	file, err := root.OpenFile(filepath.FromSlash(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func emptyPolicy(policies Policy) bool {
	return len(policies.Execution) == 0 && len(policies.Data) == 0 && len(policies.ExternalChanges) == 0
}
