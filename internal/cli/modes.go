package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"station/internal/modes"
)

func newNewCommand(root string, _ *string, result *Result, handled *bool) *cobra.Command {
	command := &cobra.Command{
		Use:   "new <idea>",
		Short: "Create a minimal Station workspace in the current directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*handled = true
			created, err := modes.CreateNewWorkspace(root, args[0], modes.NewOptions{})
			if err != nil {
				return modeCLIError(err)
			}
			*result = Result{OK: true, Status: "created", ExitCode: 0, NewWorkspace: &created}
			writeNewText(cmd.OutOrStdout(), created)
			return nil
		},
	}
	return command
}

func newInspectCommand(root string, format *string, result *Result, handled *bool) *cobra.Command {
	command := &cobra.Command{
		Use:   "inspect [path]",
		Short: "Read a shallow, policy-bounded view of an existing workspace",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateFormat(*format)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			*handled = true
			inspectionRoot := root
			if len(args) == 1 {
				if filepath.IsAbs(args[0]) {
					inspectionRoot = args[0]
				} else {
					inspectionRoot = filepath.Join(root, args[0])
				}
			}
			inspection, err := modes.InspectExistingWorkspace(inspectionRoot)
			if err != nil {
				return modeCLIError(err)
			}
			*result = Result{OK: true, Status: "inspected", ExitCode: 0, Inspection: &inspection}
			if *format == "text" {
				writeInspectionText(cmd.OutOrStdout(), inspection)
			}
			return nil
		},
	}
	command.Flags().StringVar(format, "format", "text", "Output format: text or json")
	return command
}

func modeCLIError(err error) error {
	return invalidError("%v", err)
}

func writeNewText(writer io.Writer, result modes.NewResult) {
	fmt.Fprintf(writer, "Station workspace created at %s\n", result.Root)
	fmt.Fprintln(writer, "Created files:")
	for _, file := range result.Files {
		fmt.Fprintf(writer, "- %s\n", file)
	}
}

func writeInspectionText(writer io.Writer, inspection modes.ExistingInspection) {
	fmt.Fprintf(writer, "Workspace: %s\n", inspection.Root)
	fmt.Fprintf(writer, "Manifest: %s\n", inspection.ManifestStatus)
	if len(inspection.Projects) > 0 {
		fmt.Fprintln(writer, "Projects:")
		for _, project := range inspection.Projects {
			fmt.Fprintf(writer, "- %s (%s, %s): %s\n", project.ID, project.Kind, project.Language, project.Path)
		}
	}
	if len(inspection.Entries) > 0 {
		fmt.Fprintln(writer, "Observed entries:")
		for _, entry := range inspection.Entries {
			fmt.Fprintf(writer, "- %s (%s", entry.Path, entry.Kind)
			if entry.Kind == "file" {
				fmt.Fprintf(writer, ", %d bytes", entry.Bytes)
			}
			if entry.MetadataRead {
				fmt.Fprint(writer, ", allowlisted metadata read")
			}
			fmt.Fprintln(writer, ")")
		}
	}
	for _, warning := range inspection.Warnings {
		fmt.Fprintf(writer, "Warning: %s\n", warning)
	}
}
