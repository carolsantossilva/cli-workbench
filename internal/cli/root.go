package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"station/internal/modes"
	"station/internal/workflows"
	"station/internal/workspace"
)

const Version = "0.1.0"

type Dependencies struct {
	Root   string
	Stdout io.Writer
	Stderr io.Writer
}

type Result struct {
	OK           bool                      `json:"ok"`
	Status       string                    `json:"status"`
	Code         string                    `json:"code"`
	ExitCode     int                       `json:"exitCode"`
	Diagnostics  []workspace.Diagnostic    `json:"diagnostics,omitempty"`
	NewWorkspace *modes.NewResult          `json:"newWorkspace,omitempty"`
	Inspection   *modes.ExistingInspection `json:"inspection,omitempty"`
	WorkflowRun  *WorkflowRunOutput        `json:"workflowRun,omitempty"`
	Verification *workflows.WorkflowResult `json:"verification,omitempty"`
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
		if format == "json" {
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
	command.AddCommand(newNewCommand(root, format, result, handled))
	command.AddCommand(newInspectCommand(root, format, result, handled))
	workflow := &cobra.Command{Use: "workflow", Short: "Prepare and verify workflows"}
	workflow.PersistentFlags().StringVar(format, "format", "text", "Output format: text or json")
	workflow.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return validateFormat(*format)
	}
	workflow.AddCommand(newWorkflowRunCommand(root, format, result, handled))
	workflow.AddCommand(newWorkflowVerifyCommand(root, format, result, handled))
	command.AddCommand(workflow)
	return command
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
