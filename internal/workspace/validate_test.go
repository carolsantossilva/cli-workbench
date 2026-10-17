package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadWorkspaceAcceptsValidManifest(t *testing.T) {
	root := writeFixture(t, "workspace.valid.json")

	manifest, err := loadWorkspace(root)
	if err != nil {
		t.Fatalf("loadWorkspace() error = %v", err)
	}
	if got := manifest.Workspace.ID; got != "sample-workbench" {
		t.Fatalf("workspace id = %q, want sample-workbench", got)
	}
	if diagnostics := validateWorkspace(manifest, root); len(diagnostics) != 0 {
		t.Fatalf("validateWorkspace() diagnostics = %#v, want none", diagnostics)
	}
}

func TestLoadWorkspaceReportsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.json"), []byte(`{"schemaVersion":`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadWorkspace(root)
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("loadWorkspace() error = %v, want invalid JSON", err)
	}
}

func TestLoadWorkspaceReportsSchemaDiagnosticWithPath(t *testing.T) {
	root := writeFixture(t, "workspace.invalid.json")

	_, err := loadWorkspace(root)
	if err == nil || !strings.Contains(err.Error(), "/workspace/id") {
		t.Fatalf("loadWorkspace() error = %v, want schema diagnostic at /workspace/id", err)
	}
}

func TestLoadWorkspaceRejectsUnknownSchemaVersion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.json"), []byte(`{"schemaVersion":99}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadWorkspace(root)
	if err == nil || !strings.Contains(err.Error(), "schemaVersion") {
		t.Fatalf("loadWorkspace() error = %v, want unknown schemaVersion", err)
	}
}

func TestValidateWorkspaceReportsSemanticReferences(t *testing.T) {
	root := writeFixture(t, "workspace.valid.json")
	manifest, err := loadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Workflows["brainstorm"] = Workflow{AllowedCommands: []string{"missing-command"}}

	diagnostics := validateWorkspace(manifest, root)
	if len(diagnostics) != 1 || diagnostics[0].Path != "/workflows/brainstorm/allowedCommands/0" {
		t.Fatalf("validateWorkspace() diagnostics = %#v, want missing-command diagnostic", diagnostics)
	}
}

func TestValidateWorkspaceUsesFullArrayIndexInDiagnosticPath(t *testing.T) {
	// This catches diagnostics pointing at the wrong workflow entry after index 9.
	root := writeFixture(t, "workspace.valid.json")
	manifest, err := loadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Workflows["brainstorm"] = Workflow{AllowedCommands: []string{
		"missing-0", "missing-1", "missing-2", "missing-3", "missing-4", "missing-5",
		"missing-6", "missing-7", "missing-8", "missing-9", "missing-10",
	}}

	diagnostics := validateWorkspace(manifest, root)
	if got := diagnostics[10].Path; got != "/workflows/brainstorm/allowedCommands/10" {
		t.Fatalf("diagnostics[10].Path = %q, want full array index", got)
	}
}

func TestValidateWorkspaceDiagnosticsAreDeterministicallyOrdered(t *testing.T) {
	manifest := &Workspace{
		Workspace: Identity{Kind: "monorepo"},
		Workflows: map[string]Workflow{
			"zeta": {
				AllowedCommands: []string{"missing-zeta"},
				ReadPaths:       []string{"../outside-zeta"},
				WritePaths:      []string{"../outside-write-zeta"},
			},
			"alpha": {
				AllowedCommands: []string{"missing-alpha"},
				ReadPaths:       []string{"../outside-alpha"},
				WritePaths:      []string{"../outside-write-alpha"},
			},
		},
	}
	wantPaths := []string{
		"/workflows/alpha/allowedCommands/0",
		"/workflows/alpha/readPaths/0",
		"/workflows/alpha/writePaths/0",
		"/workflows/zeta/allowedCommands/0",
		"/workflows/zeta/readPaths/0",
		"/workflows/zeta/writePaths/0",
	}

	for i := 0; i < 100; i++ {
		diagnostics := validateWorkspace(manifest, t.TempDir())
		paths := make([]string, len(diagnostics))
		for index, diagnostic := range diagnostics {
			paths[index] = diagnostic.Path
		}
		if !reflect.DeepEqual(paths, wantPaths) {
			t.Fatalf("iteration %d paths = %#v, want %#v", i, paths, wantPaths)
		}
	}
}

func writeFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
