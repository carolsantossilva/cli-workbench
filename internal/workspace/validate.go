package workspace

import (
	"path/filepath"
	"strconv"
	"strings"
)

type Diagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path"`
	Severity string `json:"severity"`
}

func validateWorkspace(manifest *Workspace, root string) []Diagnostic {
	if manifest == nil {
		return []Diagnostic{{Code: "invalid", Message: "workspace manifest is required", Path: "/", Severity: "error"}}
	}
	var diagnostics []Diagnostic
	if manifest.Workspace.Kind != "monorepo" {
		diagnostics = append(diagnostics, invalid("/workspace/kind", "workspace kind must be monorepo"))
	}
	projectIDs := make(map[string]struct{}, len(manifest.Projects))
	for i, project := range manifest.Projects {
		if _, duplicate := projectIDs[project.ID]; duplicate {
			diagnostics = append(diagnostics, invalid("/projects/"+itoa(i)+"/id", "project id is duplicated"))
		}
		projectIDs[project.ID] = struct{}{}
		if !workspaceRelativePath(project.Path) {
			diagnostics = append(diagnostics, invalid("/projects/"+itoa(i)+"/path", "project path must be relative and remain inside the workspace"))
		}
	}
	commandIDs := make(map[string]struct{}, len(manifest.Commands))
	for i, command := range manifest.Commands {
		if _, duplicate := commandIDs[command.ID]; duplicate {
			diagnostics = append(diagnostics, invalid("/commands/"+itoa(i)+"/id", "command id is duplicated"))
		}
		commandIDs[command.ID] = struct{}{}
		if _, exists := projectIDs[command.Project]; !exists {
			diagnostics = append(diagnostics, invalid("/commands/"+itoa(i)+"/project", "command project does not exist"))
		}
	}
	for id, workflow := range manifest.Workflows {
		for i, commandID := range workflow.AllowedCommands {
			if _, exists := commandIDs[commandID]; !exists {
				diagnostics = append(diagnostics, invalid("/workflows/"+id+"/allowedCommands/"+itoa(i), "workflow command does not exist"))
			}
		}
		allowedSkills := make(map[string]struct{}, len(workflow.AllowedSkills))
		for _, skill := range workflow.AllowedSkills {
			allowedSkills[skill] = struct{}{}
		}
		for i, skill := range workflow.RequiredSkills {
			if _, allowed := allowedSkills[skill]; !allowed {
				diagnostics = append(diagnostics, invalid("/workflows/"+id+"/requiredSkills/"+itoa(i), "required skill is not allowed"))
			}
		}
		for field, paths := range map[string][]string{"readPaths": workflow.ReadPaths, "writePaths": workflow.WritePaths, "inputs": workflow.Inputs, "outputs": workflow.Outputs} {
			for i, path := range paths {
				if !workspaceRelativePath(path) {
					diagnostics = append(diagnostics, invalid("/workflows/"+id+"/"+field+"/"+itoa(i), "workflow path must be relative and remain inside the workspace"))
				}
			}
		}
	}
	externalIDs := make(map[string]struct{}, len(manifest.ExternalRepositories))
	for i, repository := range manifest.ExternalRepositories {
		if _, duplicate := externalIDs[repository.ID]; duplicate {
			diagnostics = append(diagnostics, invalid("/externalRepositories/"+itoa(i)+"/id", "external repository id is duplicated"))
		}
		externalIDs[repository.ID] = struct{}{}
	}
	return diagnostics
}

func ValidateWorkspace(manifest *Workspace, root string) []Diagnostic {
	return validateWorkspace(manifest, root)
}

func invalid(path, message string) Diagnostic {
	return Diagnostic{Code: "invalid", Message: message, Path: path, Severity: "error"}
}

func workspaceRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
