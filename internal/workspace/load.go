package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"station/internal/assets"
)

const schemaURL = "station://workspace.schema.json"

type Workspace struct {
	SchemaVersion        int                  `json:"schemaVersion"`
	Workspace            Identity             `json:"workspace"`
	Projects             []Project            `json:"projects"`
	Commands             []Command            `json:"commands"`
	Policies             Policies             `json:"policies"`
	Workflows            map[string]Workflow  `json:"workflows"`
	ExternalRepositories []ExternalRepository `json:"externalRepositories"`
	Metadata             map[string]any       `json:"metadata,omitempty"`
}

type Identity struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type Project struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Language string `json:"language"`
}

type Command struct {
	ID             string   `json:"id"`
	Category       string   `json:"category"`
	Executable     string   `json:"executable"`
	Args           []string `json:"args"`
	Project        string   `json:"project"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

type Policies struct {
	Execution       map[string]any `json:"execution"`
	Data            map[string]any `json:"data"`
	ExternalChanges map[string]any `json:"externalChanges"`
}

type Workflow struct {
	RequiredSkills  []string `json:"requiredSkills"`
	AllowedSkills   []string `json:"allowedSkills"`
	AllowedCommands []string `json:"allowedCommands"`
	ReadPaths       []string `json:"readPaths"`
	WritePaths      []string `json:"writePaths"`
	Inputs          []string `json:"inputs"`
	Outputs         []string `json:"outputs"`
	Summaries       []string `json:"summaries"`
	Handoff         string   `json:"handoff"`
}

type ExternalRepository struct {
	ID               string   `json:"id"`
	Path             string   `json:"path"`
	ReadOnly         bool     `json:"readOnly"`
	AllowedReadPaths []string `json:"allowedReadPaths"`
}

var (
	compiledSchema     *jsonschema.Schema
	compiledSchemaErr  error
	compiledSchemaOnce sync.Once
)

func loadWorkspace(root string) (*Workspace, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", absoluteRoot)
	}

	content, err := os.ReadFile(filepath.Join(absoluteRoot, "workspace.json"))
	if err != nil {
		return nil, fmt.Errorf("workspace.json: %w", err)
	}
	return loadWorkspaceBytes(content)
}

func loadWorkspaceBytes(content []byte) (*Workspace, error) {
	var document any
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("invalid JSON in workspace.json: %w", err)
	}
	manifestObject, ok := document.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("workspace.json must contain a JSON object")
	}
	version, ok := manifestObject["schemaVersion"].(float64)
	if !ok || version != 1 {
		return nil, fmt.Errorf("unsupported schemaVersion")
	}
	schema, err := workspaceSchema()
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(document); err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}

	var manifest Workspace
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("decode workspace.json: %w", err)
	}
	return &manifest, nil
}

func LoadWorkspace(root string) (*Workspace, error) {
	return loadWorkspace(root)
}

// LoadWorkspaceBytes validates and decodes a workspace manifest from already
// read bytes. Callers that hold a root-anchored file handle can use this to
// avoid reopening workspace.json through an unanchored absolute path.
func LoadWorkspaceBytes(content []byte) (*Workspace, error) {
	return loadWorkspaceBytes(content)
}

func workspaceSchema() (*jsonschema.Schema, error) {
	compiledSchemaOnce.Do(func() {
		var document any
		if err := json.Unmarshal(assets.WorkspaceSchema(), &document); err != nil {
			compiledSchemaErr = fmt.Errorf("decode embedded workspace schema: %w", err)
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.UseLoader(nil)
		if err := compiler.AddResource(schemaURL, document); err != nil {
			compiledSchemaErr = fmt.Errorf("register embedded workspace schema: %w", err)
			return
		}
		compiledSchema, compiledSchemaErr = compiler.Compile(schemaURL)
	})
	return compiledSchema, compiledSchemaErr
}
