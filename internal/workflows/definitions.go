package workflows

import (
	"fmt"

	"workbench/internal/workspace"
)

// Definition is the versioned, static contract for a supported workflow.
type Definition struct {
	ID              string
	InstructionPath string
	Responsibility  string
	OutputIdentity  string
	Handoff         string
}

// Configuration contains the runtime settings copied from workspace.json.
type Configuration struct {
	RequiredSkills  []string
	AllowedSkills   []string
	AllowedCommands []string
	ReadPaths       []string
	WritePaths      []string
	Inputs          []string
	Outputs         []string
	Summaries       []string
	Handoff         string
}

// Resolved combines a static workflow definition with its workspace settings.
type Resolved struct {
	Definition    Definition
	Configuration Configuration
}

var definitions = map[string]Definition{
	"brainstorm": {
		ID:              "brainstorm",
		InstructionPath: "docs/workbench/workflows/brainstorm.md",
		Responsibility:  "produzir entendimento compartilhado, critérios e limites",
		OutputIdentity:  "docs/workbench/brief.md",
		Handoff:         "docs/workbench/brief.md",
	},
	"grill-me": {
		ID:              "grill-me",
		InstructionPath: "docs/workbench/workflows/grill-me.md",
		Responsibility:  "desafiar ambiguidades, premissas, riscos e casos extremos",
		OutputIdentity:  "docs/workbench/decisions.md e design seguinte",
		Handoff:         "docs/workbench/decisions.md",
	},
	"design": {
		ID:              "design",
		InstructionPath: "docs/workbench/workflows/design.md",
		Responsibility:  "registrar arquitetura, fluxo, erros, testes e decisões do próximo incremento",
		OutputIdentity:  "docs/workbench/design.md",
		Handoff:         "docs/workbench/design.md",
	},
	"build": {
		ID:              "build",
		InstructionPath: "docs/workbench/workflows/build.md",
		Responsibility:  "dividir execução em lotes, aplicar política e verificar diffs/testes",
		OutputIdentity:  "docs/workbench/plan.md, arquivos do lote e resultados reais",
		Handoff:         "docs/workbench/plan.md + paths alterados + design.md",
	},
	"review": {
		ID:              "review",
		InstructionPath: "docs/workbench/workflows/review.md",
		Responsibility:  "verificar aderência, segurança e políticas de dados",
		OutputIdentity:  "resultado em decisions.md",
		Handoff:         "snapshot/diff + testes + decisions.md",
	},
}

// Resolve returns a supported workflow definition and its copied runtime config.
// It rejects unsupported or missing workflows and invalid required-skill sets.
func Resolve(id string, manifest *workspace.Workspace) (Resolved, error) {
	definition, supported := definitions[id]
	if !supported {
		return Resolved{}, fmt.Errorf("unsupported workflow %q", id)
	}
	if manifest == nil {
		return Resolved{}, fmt.Errorf("workflow %q requires a workspace manifest", id)
	}

	workflow, exists := manifest.Workflows[id]
	if !exists {
		return Resolved{}, fmt.Errorf("workflow %q is missing from workspace manifest", id)
	}
	if err := validateRequiredSkills(id, workflow); err != nil {
		return Resolved{}, err
	}

	return Resolved{
		Definition: definition,
		Configuration: Configuration{
			RequiredSkills:  cloneStrings(workflow.RequiredSkills),
			AllowedSkills:   cloneStrings(workflow.AllowedSkills),
			AllowedCommands: cloneStrings(workflow.AllowedCommands),
			ReadPaths:       cloneStrings(workflow.ReadPaths),
			WritePaths:      cloneStrings(workflow.WritePaths),
			Inputs:          cloneStrings(workflow.Inputs),
			Outputs:         cloneStrings(workflow.Outputs),
			Summaries:       cloneStrings(workflow.Summaries),
			Handoff:         workflow.Handoff,
		},
	}, nil
}

func validateRequiredSkills(id string, workflow workspace.Workflow) error {
	allowed := make(map[string]struct{}, len(workflow.AllowedSkills))
	for _, skill := range workflow.AllowedSkills {
		allowed[skill] = struct{}{}
	}
	for _, skill := range workflow.RequiredSkills {
		if _, ok := allowed[skill]; !ok {
			return fmt.Errorf("workflow %q requires skill %q that is not allowed", id, skill)
		}
	}
	return nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
