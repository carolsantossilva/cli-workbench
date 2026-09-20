package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"station/internal/workspace"
)

const Version = "0.1.0"

type Dependencies struct {
	Root   string
	Stdout io.Writer
	Stderr io.Writer
}

type Result struct {
	OK          bool                   `json:"ok"`
	Status      string                 `json:"status"`
	Code        string                 `json:"code"`
	ExitCode    int                    `json:"exitCode"`
	Diagnostics []workspace.Diagnostic `json:"diagnostics,omitempty"`
}

func runCLI(ctx context.Context, argv []string, deps Dependencies) Result {
	stdout := deps.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := deps.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	root := deps.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return failedResult(invalidError("resolve current directory: %v", err), stderr)
		}
	}

	command := newRootCommand(root)
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetArgs(argv)
	if err := command.ExecuteContext(ctx); err != nil {
		return failedResult(err, stderr)
	}
	return Result{OK: true, Status: "ready", ExitCode: 0}
}

func Run(ctx context.Context, argv []string, deps Dependencies) Result {
	return runCLI(ctx, argv, deps)
}

func newRootCommand(root string) *cobra.Command {
	command := &cobra.Command{
		Use:           "station",
		Short:         "Station workspace contract CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       Version,
	}
	command.AddCommand(newValidateCommand(root))
	command.AddCommand(unavailableCommand("new <idea>"))
	command.AddCommand(unavailableCommand("inspect [path]"))
	workflow := &cobra.Command{Use: "workflow", Short: "Prepare and verify workflows"}
	workflow.AddCommand(unavailableCommand("run <workflow-id>"))
	workflow.AddCommand(unavailableCommand("verify <workflow-id> <run-id>"))
	command.AddCommand(workflow)
	return command
}

func unavailableCommand(use string) *cobra.Command {
	return &cobra.Command{
		Use:  use,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return &Error{Code: CodePrerequisiteMissing, Message: "command is not available in phase 1"}
		},
	}
}

func failedResult(err error, stderr io.Writer) Result {
	code := CodeInvalid
	if typed, ok := err.(*Error); ok {
		code = typed.Code
	}
	fmt.Fprintln(stderr, err)
	return Result{Status: "invalid", Code: string(code), ExitCode: exitCode(code)}
}
