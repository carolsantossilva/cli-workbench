package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"station/internal/workspace"
)

func newValidateCommand(root string, format *string, result *Result, handled *bool) *cobra.Command {
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate workspace.json without writing files",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateFormat(*format)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			*handled = true
			manifest, err := workspace.LoadWorkspace(root)
			if err != nil {
				diagnostic := workspace.DiagnosticFromError(err)
				return &Error{
					Code:        CodeInvalid,
					Status:      "invalid",
					Message:     err.Error(),
					Diagnostics: []workspace.Diagnostic{diagnostic},
				}
			}
			diagnostics := workspace.ValidateWorkspace(manifest, root)
			if len(diagnostics) > 0 {
				return &Error{
					Code:        CodeInvalid,
					Status:      "invalid",
					Message:     fmt.Sprintf("workspace validation failed at %s: %s", diagnostics[0].Path, diagnostics[0].Message),
					Diagnostics: diagnostics,
				}
			}
			*result = Result{OK: true, Status: "ready", ExitCode: 0}
			if *format == "text" {
				fmt.Fprintln(cmd.OutOrStdout(), "workspace.json is valid")
			}
			return nil
		},
	}
	command.Flags().StringVar(format, "format", "text", "Output format: text or json")
	return command
}
