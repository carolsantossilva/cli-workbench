package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"station/internal/workspace"
)

func newValidateCommand(root string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate workspace.json without writing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := workspace.LoadWorkspace(root)
			if err != nil {
				return invalidError("%v", err)
			}
			diagnostics := workspace.ValidateWorkspace(manifest, root)
			if len(diagnostics) > 0 {
				return invalidError("workspace validation failed at %s: %s", diagnostics[0].Path, diagnostics[0].Message)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "workspace.json is valid")
			return nil
		},
	}
}
