package cli

import (
	"context"
	"encoding/json"
	"errors"
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
			cliErr := invalidError("resolve current directory: %v", err)
			fmt.Fprintln(stderr, cliErr)
			return failedResult(cliErr)
		}
	}

	format := "text"
	handled := false
	result := Result{}
	command := newRootCommand(root, &format, &result, &handled)
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetArgs(argv)
	if err := command.ExecuteContext(ctx); err != nil {
		result = failedResult(err)
		if format == "json" && handled {
			writeResultJSON(stdout, result)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return result
	}
	if result.Status == "" {
		result = Result{OK: true, Status: "ready", ExitCode: 0}
	}
	if format == "json" && handled {
		writeResultJSON(stdout, result)
	}
	return result
}

func Run(ctx context.Context, argv []string, deps Dependencies) Result {
	return runCLI(ctx, argv, deps)
}

func newRootCommand(root string, format *string, result *Result, handled *bool) *cobra.Command {
	command := &cobra.Command{
		Use:           "station",
		Short:         "Station workspace contract CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       Version,
	}
	command.AddCommand(newValidateCommand(root, format, result, handled))
	command.AddCommand(unavailableCommand("new <idea>", nil))
	command.AddCommand(unavailableCommand("inspect [path]", nil))
	workflow := &cobra.Command{Use: "workflow", Short: "Prepare and verify workflows"}
	workflow.PersistentFlags().StringVar(format, "format", "text", "Output format: text or json")
	workflow.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return validateFormat(*format)
	}
	workflow.AddCommand(unavailableCommand("run <workflow-id>", handled))
	workflow.AddCommand(unavailableCommand("verify <workflow-id> <run-id>", handled))
	command.AddCommand(workflow)
	return command
}

func unavailableCommand(use string, handled *bool) *cobra.Command {
	return &cobra.Command{
		Use:  use,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled != nil {
				*handled = true
			}
			return &Error{Code: CodePrerequisiteMissing, Status: "blocked", Message: "command is not available in phase 1"}
		},
	}
}

func failedResult(err error) Result {
	code := CodeInvalid
	status := "invalid"
	var diagnostics []workspace.Diagnostic
	var typed *Error
	if errors.As(err, &typed) {
		code = typed.Code
		if typed.Status != "" {
			status = typed.Status
		}
		diagnostics = typed.Diagnostics
	}
	if status == "invalid" {
		switch code {
		case CodeAsk:
			status = "ask"
		case CodeDeny, CodeArtifactMissing, CodePrerequisiteMissing:
			status = "blocked"
		}
	}
	return Result{Status: status, Code: string(code), ExitCode: exitCode(code), Diagnostics: diagnostics}
}

func validateFormat(format string) error {
	if format != "text" && format != "json" {
		return invalidError("invalid argument --format: %q (expected text or json)", format)
	}
	return nil
}

func writeResultJSON(w io.Writer, result Result) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(result)
}
