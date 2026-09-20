package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"station/internal/assets"
	"station/internal/cli"
	"station/internal/policy"
	"station/internal/workspace"
)

func TestNewCreatesMinimalContractualMonorepoWithoutInstallingDependencies(t *testing.T) {
	root := t.TempDir()
	trapMarker := installExternalCommandTrap(t)
	var stdout, stderr bytes.Buffer
	result := cli.Run(context.Background(), []string{"new", "A personal planner idea"}, cli.Dependencies{
		Root: root, Stdout: &stdout, Stderr: &stderr,
	})
	if !result.OK || result.ExitCode != 0 {
		t.Fatalf("station new failed: result=%#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("station new wrote to stderr: %q", stderr.String())
	}

	manifest, err := workspace.LoadWorkspace(root)
	if err != nil {
		t.Fatalf("generated workspace.json is invalid: %v", err)
	}
	if diagnostics := workspace.ValidateWorkspace(manifest, root); len(diagnostics) != 0 {
		t.Fatalf("generated workspace has semantic diagnostics: %#v", diagnostics)
	}
	if diagnostics := policy.ValidateManifestPolicies(manifest); len(diagnostics) != 0 {
		t.Fatalf("generated workspace has policy diagnostics: %#v", diagnostics)
	}

	wantProjects := []workspace.Project{
		{ID: "mobile", Path: "apps/mobile", Kind: "mobile", Language: "typescript"},
		{ID: "web", Path: "apps/web", Kind: "web", Language: "typescript"},
		{ID: "server", Path: "apps/server", Kind: "server", Language: "go"},
	}
	if !reflect.DeepEqual(manifest.Projects, wantProjects) {
		t.Fatalf("projects = %#v, want %#v", manifest.Projects, wantProjects)
	}
	if manifest.Workspace.Kind != "monorepo" || len(manifest.Commands) != 0 || len(manifest.ExternalRepositories) != 0 {
		t.Fatalf("generated workspace identity/commands/external repositories = %#v, %#v, %#v", manifest.Workspace, manifest.Commands, manifest.ExternalRepositories)
	}
	if got := manifest.Policies.Execution["default"]; got != "ask" {
		t.Fatalf("execution default = %#v, want ask", got)
	}
	dataPolicy, err := policy.ParseDataPolicy(manifest.Policies.Data)
	if err != nil {
		t.Fatalf("generated data policy is invalid: %v", err)
	}
	for _, test := range []struct {
		category string
		want     policy.Outcome
	}{
		{category: "design", want: policy.Allow},
		{category: "source", want: policy.Ask},
		{category: "test-output", want: policy.Ask},
		{category: "secret", want: policy.Deny},
		{category: "unknown", want: policy.Ask},
	} {
		got := policy.ScanData("docs/station/idea.md", "fixture", test.category, dataPolicy).Decision.Outcome
		if got != test.want {
			t.Errorf("data category %q decision = %q, want %q", test.category, got, test.want)
		}
	}

	wantContracts := map[string]struct {
		inputs  []string
		output  string
		handoff string
	}{
		"brainstorm": {inputs: []string{"docs/station/idea.md"}, output: "docs/station/brief.md", handoff: "docs/station/brief.md"},
		"grill-me":   {inputs: []string{"docs/station/brief.md"}, output: "docs/station/decisions.md", handoff: "docs/station/decisions.md"},
		"design":     {inputs: []string{"docs/station/brief.md", "docs/station/decisions.md"}, output: "docs/station/design.md", handoff: "docs/station/design.md"},
		"build":      {inputs: []string{"docs/station/design.md"}, output: "docs/station/plan.md", handoff: "docs/station/plan.md + paths alterados + design.md"},
		"review":     {inputs: []string{"docs/station/decisions.md", "docs/station/design.md", "docs/station/plan.md"}, output: "docs/station/decisions.md", handoff: "snapshot/diff + testes + decisions.md"},
	}
	wantWorkflowIDs := []string{"brainstorm", "build", "design", "grill-me", "review"}
	gotWorkflowIDs := make([]string, 0, len(manifest.Workflows))
	for id, workflow := range manifest.Workflows {
		gotWorkflowIDs = append(gotWorkflowIDs, id)
		contract, ok := wantContracts[id]
		if !ok {
			t.Errorf("unexpected workflow ID %q", id)
			continue
		}
		if len(workflow.RequiredSkills) != 0 || len(workflow.AllowedCommands) != 0 {
			t.Errorf("workflow %q unexpectedly requires skills or allows commands: %#v", id, workflow)
		}
		if !reflect.DeepEqual(workflow.AllowedSkills, []string{"station-mobile", "station-web", "station-server"}) {
			t.Errorf("workflow %q allowedSkills = %#v", id, workflow.AllowedSkills)
		}
		if !reflect.DeepEqual(workflow.ReadPaths, []string{"docs/station/**"}) {
			t.Errorf("workflow %q readPaths = %#v", id, workflow.ReadPaths)
		}
		if !reflect.DeepEqual(workflow.WritePaths, workflow.Outputs) || len(workflow.Outputs) != 1 {
			t.Errorf("workflow %q outputs/writePaths = %#v/%#v", id, workflow.Outputs, workflow.WritePaths)
		}
		if !reflect.DeepEqual(workflow.Inputs, contract.inputs) || !reflect.DeepEqual(workflow.Outputs, []string{contract.output}) || workflow.Handoff != contract.handoff {
			t.Errorf("workflow %q contract inputs/output/handoff = %#v/%#v/%q", id, workflow.Inputs, workflow.Outputs, workflow.Handoff)
		}
	}
	sort.Strings(gotWorkflowIDs)
	if !reflect.DeepEqual(gotWorkflowIDs, wantWorkflowIDs) {
		t.Fatalf("workflow IDs = %#v, want %#v", gotWorkflowIDs, wantWorkflowIDs)
	}

	for _, name := range []string{
		"README.md", "AGENTS.md", "apps/mobile/README.md", "apps/mobile/AGENTS.md",
		"apps/web/README.md", "apps/web/AGENTS.md", "apps/server/README.md", "apps/server/AGENTS.md",
		"docs/station/idea.md", "schemas/workspace.schema.json",
		"docs/station/workflows/brainstorm.md", "docs/station/workflows/grill-me.md",
		"docs/station/workflows/design.md", "docs/station/workflows/build.md", "docs/station/workflows/review.md",
		".agents/skills/station-mobile/SKILL.md", ".agents/skills/station-web/SKILL.md", ".agents/skills/station-server/SKILL.md",
	} {
		if info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name))); err != nil || !info.Mode().IsRegular() {
			t.Errorf("expected generated regular file %s, info=%v err=%v", name, info, err)
		}
	}

	schema, err := os.ReadFile(filepath.Join(root, "schemas", "workspace.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(schema, assets.WorkspaceSchema()) {
		t.Fatal("generated schema differs from the canonical embedded schema")
	}
	idea, err := os.ReadFile(filepath.Join(root, "docs", "station", "idea.md"))
	if err != nil || string(idea) != "A personal planner idea\n" {
		t.Fatalf("idea input = %q, err=%v", idea, err)
	}

	for _, name := range []string{"apps/server/go.mod", "apps/server/go.sum", "apps/mobile/package.json", "apps/web/package.json", "packages/shared", "node_modules", ".git"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("new workspace unexpectedly created %s: %v", name, err)
		}
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"commands":[]`)) || !bytes.Contains(encoded, []byte(`"externalRepositories":[]`)) {
		t.Fatalf("commands or externalRepositories are not explicitly empty: %s", encoded)
	}
	if _, err := os.Stat(trapMarker); !os.IsNotExist(err) {
		t.Fatalf("station new invoked an external generator or command: %v", err)
	}
}

func TestNewWorkspaceCanPrepareInitialBuildFromApprovedDesign(t *testing.T) {
	root := t.TempDir()
	created := cli.Run(context.Background(), []string{"new", "generic workflow fixture"}, cli.Dependencies{Root: root})
	if !created.OK {
		t.Fatalf("station new failed: %#v", created)
	}
	designPath := filepath.Join(root, "docs", "station", "design.md")
	if err := os.WriteFile(designPath, []byte("# Approved design fixture\n"), 0o600); err != nil {
		t.Fatalf("write approved design fixture: %v", err)
	}

	prepared := cli.Run(context.Background(), []string{"workflow", "run", "build", "--format", "json"}, cli.Dependencies{Root: root})
	if !prepared.OK || prepared.Status != "prepared" || prepared.WorkflowRun == nil {
		t.Fatalf("initial build workflow could not be prepared from its approved design: %#v", prepared)
	}
	inputs := prepared.WorkflowRun.Context.Inputs
	if len(inputs) != 1 || inputs[0].Path != "docs/station/design.md" {
		t.Fatalf("initial build inputs = %#v, want only the existing approved design", inputs)
	}
	outputs := prepared.WorkflowRun.Context.ExpectedOutputs
	if len(outputs) != 1 || outputs[0].Path != "docs/station/plan.md" || outputs[0].Exists || outputs[0].Status != "pending" {
		t.Fatalf("initial build expected outputs = %#v, want a pending plan artifact", outputs)
	}
}

func TestNewRefusesNonEmptyDestinationWithoutOverwritingOrAddingFiles(t *testing.T) {
	root := t.TempDir()
	markerPath := filepath.Join(root, "keep.txt")
	const marker = "existing content must survive exactly"
	if err := os.WriteFile(markerPath, []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	result := cli.Run(context.Background(), []string{"new", "do not overwrite"}, cli.Dependencies{
		Root: root, Stdout: &stdout, Stderr: &stderr,
	})
	if result.OK || result.ExitCode == 0 {
		t.Fatalf("station new accepted a non-empty destination: result=%#v stdout=%q", result, stdout.String())
	}
	content, err := os.ReadFile(markerPath)
	if err != nil || string(content) != marker {
		t.Fatalf("existing file changed: content=%q err=%v", content, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "keep.txt" {
		t.Fatalf("non-empty destination changed: entries=%v", entries)
	}
}

func installExternalCommandTrap(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable trap is not available on Windows")
	}
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "invoked.txt")
	marker = strings.ReplaceAll(marker, "'", "'\\''")
	for _, name := range []string{"npm", "npx", "expo", "vite", "go", "git"} {
		script := "#!/bin/sh\nprintf '%s\\n' '" + name + "' >> '" + marker + "'\nexit 97\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	return marker
}
