package policy

import (
	"path"
	"strings"

	"station/internal/workspace"
)

type Command = workspace.Command

func authorizeCommand(commandID, workflowID string, manifest *workspace.Workspace) (Decision, Command) {
	if manifest == nil {
		return decision(Deny, "workspace manifest is unavailable", "manifest"), Command{}
	}
	command, exists := commandByID(manifest, commandID)
	if !exists {
		return decision(Deny, "command is not declared", "command"), Command{}
	}
	if strings.TrimSpace(command.Executable) == "" || strings.ContainsRune(command.Executable, '\x00') || strings.TrimSpace(command.Executable) != command.Executable {
		return decision(Deny, "command executable is invalid", "command.executable"), Command{}
	}
	for _, arg := range command.Args {
		if strings.ContainsRune(arg, '\x00') {
			return decision(Deny, "command argument is invalid", "command.args"), Command{}
		}
	}
	if isShell(command.Executable, command.Args) {
		return decision(Deny, "shell executables are not allowed", "no-shell"), Command{}
	}
	action := Action{Kind: ActionCommand, CommandID: commandID, ProjectID: command.Project}
	return authorize(action, manifest, workflowID), command
}

func AuthorizeCommand(commandID, workflowID string, manifest *workspace.Workspace) (Decision, Command) {
	return authorizeCommand(commandID, workflowID, manifest)
}

func isShell(executable string, args []string) bool {
	name := strings.ToLower(path.Base(strings.ReplaceAll(executable, "\\", "/")))
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "sh", "bash", "dash", "zsh", "fish", "ksh", "csh", "tcsh", "cmd", "powershell", "pwsh":
		return true
	case "busybox":
		return len(args) > 0 && strings.EqualFold(args[0], "sh")
	default:
		return false
	}
}
