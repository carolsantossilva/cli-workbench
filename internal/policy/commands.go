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
	if isShellName(name) {
		return true
	}
	switch name {
	case "busybox":
		if len(args) > 0 && strings.EqualFold(args[0], "env") {
			return envInvokesShell(args[1:])
		}
		return len(args) > 0 && isShellName(args[0])
	case "env":
		return envInvokesShell(args)
	default:
		return false
	}
}

func isShellName(value string) bool {
	name := strings.ToLower(path.Base(strings.ReplaceAll(value, "\\", "/")))
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "sh", "ash", "bash", "dash", "zsh", "fish", "ksh", "csh", "tcsh", "cmd", "powershell", "pwsh":
		return true
	default:
		return false
	}
}

func envInvokesShell(args []string) bool {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "-S" || argument == "--split-string":
			// env applies shell-like tokenization here; conservatively reject the
			// wrapper rather than let quoting or escapes hide a shell executable.
			return true
		case argument == "-u" || argument == "--unset" || argument == "-C" || argument == "--chdir" || argument == "--argv0":
			index++
			continue
		case strings.HasPrefix(argument, "--unset=") || strings.HasPrefix(argument, "--chdir=") || strings.HasPrefix(argument, "--argv0="):
			continue
		case argument == "-i" || argument == "--ignore-environment" || argument == "-0" || argument == "--null":
			continue
		case strings.HasPrefix(argument, "-"):
			continue
		case strings.Contains(argument, "=") && !strings.HasPrefix(argument, "="):
			continue
		default:
			return isShellName(argument)
		}
	}
	return false
}
