package workflows

import (
	"reflect"
	"testing"

	"station/internal/workspace"
)

func TestResolveReturnsApprovedWorkflowDefinitions(t *testing.T) {
	tests := []struct {
		id             string
		responsibility string
		outputIdentity string
		handoff        string
	}{
		{
			id:             "brainstorm",
			responsibility: "produzir entendimento compartilhado, critérios e limites",
			outputIdentity: "docs/station/brief.md",
			handoff:        "docs/station/brief.md",
		},
		{
			id:             "grill-me",
			responsibility: "desafiar ambiguidades, premissas, riscos e casos extremos",
			outputIdentity: "docs/station/decisions.md e design seguinte",
			handoff:        "docs/station/decisions.md",
		},
		{
			id:             "design",
			responsibility: "registrar arquitetura, fluxo, erros, testes e decisões do próximo incremento",
			outputIdentity: "docs/station/design.md",
			handoff:        "docs/station/design.md",
		},
		{
			id:             "build",
			responsibility: "dividir execução em lotes, aplicar política e verificar diffs/testes",
			outputIdentity: "docs/station/plan.md, arquivos do lote e resultados reais",
			handoff:        "docs/station/plan.md + paths alterados + design.md",
		},
		{
			id:             "review",
			responsibility: "verificar aderência, segurança e políticas de dados",
			outputIdentity: "resultado em decisions.md",
			handoff:        "snapshot/diff + testes + decisions.md",
		},
	}

	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			manifest := &workspace.Workspace{Workflows: map[string]workspace.Workflow{
				test.id: {},
			}}

			resolved, err := Resolve(test.id, manifest)
			if err != nil {
				t.Fatalf("Resolve(%q) returned error: %v", test.id, err)
			}
			if resolved.Definition.ID != test.id {
				t.Errorf("definition ID = %q, want %q", resolved.Definition.ID, test.id)
			}
			if resolved.Definition.InstructionPath != "docs/station/workflows/"+test.id+".md" {
				t.Errorf("instruction path = %q", resolved.Definition.InstructionPath)
			}
			if resolved.Definition.Responsibility != test.responsibility {
				t.Errorf("responsibility = %q, want %q", resolved.Definition.Responsibility, test.responsibility)
			}
			if resolved.Definition.OutputIdentity != test.outputIdentity {
				t.Errorf("output identity = %q, want %q", resolved.Definition.OutputIdentity, test.outputIdentity)
			}
			if resolved.Definition.Handoff != test.handoff {
				t.Errorf("handoff = %q, want %q", resolved.Definition.Handoff, test.handoff)
			}
		})
	}
}

func TestResolveCopiesManifestWorkflowConfiguration(t *testing.T) {
	source := workspace.Workflow{
		RequiredSkills:  []string{"station-web"},
		AllowedSkills:   []string{"station-web", "station-server"},
		AllowedCommands: []string{"web.test"},
		ReadPaths:       []string{"apps/web"},
		WritePaths:      []string{"apps/web/src"},
		Inputs:          []string{"docs/station/design.md"},
		Outputs:         []string{"apps/web/src/App.tsx"},
		Summaries:       []string{"git.diff"},
		Handoff:         "docs/station/review-input.md",
	}
	manifest := &workspace.Workspace{Workflows: map[string]workspace.Workflow{"build": source}}

	resolved, err := Resolve("build", manifest)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := Configuration{
		RequiredSkills:  []string{"station-web"},
		AllowedSkills:   []string{"station-web", "station-server"},
		AllowedCommands: []string{"web.test"},
		ReadPaths:       []string{"apps/web"},
		WritePaths:      []string{"apps/web/src"},
		Inputs:          []string{"docs/station/design.md"},
		Outputs:         []string{"apps/web/src/App.tsx"},
		Summaries:       []string{"git.diff"},
		Handoff:         "docs/station/review-input.md",
	}
	if !reflect.DeepEqual(resolved.Configuration, want) {
		t.Fatalf("configuration = %#v, want %#v", resolved.Configuration, want)
	}

	resolved.Configuration.RequiredSkills[0] = "changed-in-result"
	resolved.Configuration.AllowedSkills[0] = "changed-in-result"
	resolved.Configuration.AllowedCommands[0] = "changed-in-result"
	resolved.Configuration.ReadPaths[0] = "changed-in-result"
	resolved.Configuration.WritePaths[0] = "changed-in-result"
	resolved.Configuration.Inputs[0] = "changed-in-result"
	resolved.Configuration.Outputs[0] = "changed-in-result"
	resolved.Configuration.Summaries[0] = "changed-in-result"
	if !reflect.DeepEqual(manifest.Workflows["build"], source) {
		t.Fatalf("mutating resolved configuration changed the manifest: %#v", manifest.Workflows["build"])
	}

	manifest.Workflows["build"] = workspace.Workflow{RequiredSkills: []string{"changed-in-manifest"}}
	if resolved.Configuration.RequiredSkills[0] != "changed-in-result" {
		t.Fatalf("mutating manifest changed resolved configuration: %#v", resolved.Configuration.RequiredSkills)
	}
}

func TestResolveRejectsInvalidWorkflowRequests(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		manifest *workspace.Workspace
	}{
		{
			name: "nil manifest",
			id:   "brainstorm",
		},
		{
			name:     "unsupported workflow",
			id:       "deploy",
			manifest: &workspace.Workspace{Workflows: map[string]workspace.Workflow{"deploy": {}}},
		},
		{
			name:     "missing runtime workflow",
			id:       "brainstorm",
			manifest: &workspace.Workspace{Workflows: map[string]workspace.Workflow{}},
		},
		{
			name: "required skill is not allowed",
			id:   "build",
			manifest: &workspace.Workspace{Workflows: map[string]workspace.Workflow{
				"build": {RequiredSkills: []string{"station-web"}, AllowedSkills: []string{"station-server"}},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Resolve(test.id, test.manifest); err == nil {
				t.Fatalf("Resolve(%q) unexpectedly succeeded", test.id)
			}
		})
	}
}
