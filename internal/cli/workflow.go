package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"workbench/internal/audit"
	"workbench/internal/policy"
	"workbench/internal/workflows"
	"workbench/internal/workspace"
)

const maxCommandOutputBytes = 64 << 10
const commandWaitDelay = time.Second

type WorkflowRunOutput struct {
	RunID   string                  `json:"runId"`
	Context workflows.ContextPacket `json:"context"`
	Command *WorkflowCommandOutput  `json:"command,omitempty"`
}

type WorkflowCommandOutput struct {
	ID            string `json:"id"`
	ExitCode      int    `json:"exitCode"`
	Stdout        string `json:"-"`
	Stderr        string `json:"-"`
	StdoutSHA256  string `json:"stdoutSha256,omitempty"`
	StderrSHA256  string `json:"stderrSha256,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
	OutputOmitted bool   `json:"outputOmitted,omitempty"`
}

func newWorkflowRunCommand(root string, format *string, result *Result, handled *bool) *cobra.Command {
	commandID := ""
	command := &cobra.Command{
		Use:   "run <workflow-id>",
		Short: "Prepare a workflow context and handoff",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*handled = true
			packet, runID, err := workflows.PrepareWorkflow(root, args[0], workflows.Input{})
			if err != nil {
				return workflowCLIError(err)
			}
			output := WorkflowRunOutput{RunID: runID, Context: packet}
			if commandID != "" {
				commandOutput, err := executeWorkflowCommand(cmd.Context(), root, args[0], runID, commandID)
				if err != nil {
					return workflowCLIError(err)
				}
				output.Command = commandOutput
			}
			status, code, exit := "prepared", "", 0
			if output.Command != nil && output.Command.ExitCode != 0 {
				status, code, exit = "command-failed", "command-failed", output.Command.ExitCode
			}
			*result = Result{OK: exit == 0, Status: status, Code: code, ExitCode: exit, WorkflowRun: &output}
			if *format == "text" {
				writeWorkflowRunText(cmd.OutOrStdout(), output)
			}
			return nil
		},
	}
	command.Flags().StringVar(&commandID, "command", "", "Run a declared, workflow-allowed command by ID")
	return command
}

func newWorkflowVerifyCommand(root string, format *string, result *Result, handled *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "verify <workflow-id> <run-id>",
		Short: "Verify workflow outputs and observable changes",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			*handled = true
			verified, err := workflows.VerifyWorkflow(root, args[0], args[1])
			if err != nil {
				return workflowCLIError(err)
			}
			*result = Result{OK: verified.Status == "completed", Status: verified.Status, ExitCode: 0, Verification: &verified}
			if *format == "text" {
				writeWorkflowVerifyText(cmd.OutOrStdout(), verified)
			}
			return nil
		},
	}
}

func workflowCLIError(err error) error {
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return err
	}
	var workflowErr *workflows.WorkflowError
	if !errors.As(err, &workflowErr) {
		return &Error{Code: CodePrerequisiteMissing, Status: "blocked", Message: "workflow prerequisite failed"}
	}
	code := CodeInvalid
	status := "invalid"
	switch workflowErr.Code {
	case workflows.FailureAsk:
		code, status = CodeAsk, "ask"
	case workflows.FailureDeny:
		code, status = CodeDeny, "blocked"
	case workflows.FailureArtifactMissing:
		code, status = CodeArtifactMissing, "blocked"
	case workflows.FailurePrerequisiteMissing:
		code, status = CodePrerequisiteMissing, "blocked"
	}
	message := workflowErr.Message
	if workflowErr.Path != "" {
		message = workflowErr.Path + ": " + message
	}
	path := workflowErr.Path
	if path == "" {
		path = "/"
	}
	return &Error{
		Code:    code,
		Status:  status,
		Message: message,
		Diagnostics: []workspace.Diagnostic{{
			Code: string(workflowErr.Code), Message: workflowErr.Message, Path: path, Severity: "error",
		}},
	}
}

func executeWorkflowCommand(ctx context.Context, root, workflowID, runID, commandID string) (*WorkflowCommandOutput, error) {
	manifest, err := workspace.LoadWorkspace(root)
	if err != nil {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailureInvalid, Path: "workspace.json", Message: "workspace manifest is invalid or missing"})
	}
	if err := workflows.ValidatePreparedRunManifest(root, workflowID, runID, manifest); err != nil {
		return nil, workflowCLIError(err)
	}
	decision, command := policy.AuthorizeCommand(commandID, workflowID, manifest)
	project, found := projectByID(manifest, command.Project)
	paths := []string{}
	if found {
		paths = append(paths, project.Path)
	}
	if _, _, err := audit.Append(root, audit.Event{
		Kind: audit.KindCommand, Decision: auditDecision(decision.Outcome), RunID: runID,
		Workflow: workflowID, CommandID: commandID, Paths: paths,
	}); err != nil {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailurePrerequisiteMissing, Path: ".workbench/audit.jsonl", Message: "could not audit command authorization"})
	}
	if decision.Outcome != policy.Allow {
		code := workflows.FailureDeny
		if decision.Outcome == policy.Ask {
			code = workflows.FailureAsk
		}
		return nil, workflowCLIError(&workflows.WorkflowError{Code: code, Path: commandID, Message: "command is not allowed by the workspace policy"})
	}
	if !found {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailureInvalid, Path: commandID, Message: "command project is missing"})
	}
	projectPath, _, err := policy.AssertWorkspacePath(root, project.Path, policy.PathRead)
	if err != nil {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailureDeny, Path: project.Path, Message: "command project is outside the workspace"})
	}
	info, err := os.Stat(projectPath)
	if err != nil || !info.IsDir() {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailurePrerequisiteMissing, Path: project.Path, Message: "command project directory is unavailable"})
	}
	if command.TimeoutSeconds < 1 || int64(command.TimeoutSeconds) > math.MaxInt64/int64(time.Second) {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailureInvalid, Path: commandID, Message: "command timeout is invalid"})
	}
	if filepath.IsAbs(command.Executable) {
		if strings.ContainsRune(command.Executable, 0) {
			return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailureInvalid, Path: commandID, Message: "command executable is invalid"})
		}
	} else if strings.ContainsAny(command.Executable, `/\\`) {
		if !filepath.IsLocal(command.Executable) {
			return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailureDeny, Path: commandID, Message: "relative command executable escapes its project"})
		}
		command.Executable = filepath.Join(projectPath, command.Executable)
	}
	commandContext, cancel := context.WithTimeout(ctx, time.Duration(command.TimeoutSeconds)*time.Second)
	defer cancel()
	process := exec.CommandContext(commandContext, command.Executable, command.Args...)
	process.Dir = projectPath
	process.WaitDelay = commandWaitDelay
	stdout, stderr := &boundedCapture{}, &boundedCapture{}
	process.Stdout, process.Stderr = stdout, stderr
	runErr := process.Run()
	exitCode := 0
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			exitCode = exitError.ExitCode()
			if exitCode < 0 {
				exitCode = 1
			}
		} else {
			exitCode = 1
		}
		if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
			return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailurePrerequisiteMissing, Path: commandID, Message: "command timed out"})
		}
	}
	dataPolicy, err := policy.ParseDataPolicy(manifest.Policies.Data)
	if err != nil {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailureInvalid, Path: "/policies/data", Message: "data policy is invalid"})
	}
	dataPolicy.ProjectID = command.Project
	output := &WorkflowCommandOutput{ID: commandID, ExitCode: exitCode, Truncated: stdout.truncated || stderr.truncated}
	stdoutAllowed := safeCommandOutput(stdout.data) && policy.ScanData(project.Path, string(stdout.data), "test-output", dataPolicy).Decision.Outcome == policy.Allow
	stderrAllowed := safeCommandOutput(stderr.data) && policy.ScanData(project.Path, string(stderr.data), "test-output", dataPolicy).Decision.Outcome == policy.Allow
	if stdoutAllowed {
		output.Stdout = string(stdout.data)
		output.StdoutSHA256 = contentHash(stdout.data)
	} else if len(stdout.data) > 0 {
		output.OutputOmitted = true
	}
	if stderrAllowed {
		output.Stderr = string(stderr.data)
		output.StderrSHA256 = contentHash(stderr.data)
	} else if len(stderr.data) > 0 {
		output.OutputOmitted = true
	}
	var hashes []string
	if output.StdoutSHA256 != "" {
		hashes = append(hashes, output.StdoutSHA256)
	}
	if output.StderrSHA256 != "" {
		hashes = append(hashes, output.StderrSHA256)
	}
	if _, _, err := audit.Append(root, audit.Event{
		Kind: audit.KindCommand, Decision: audit.DecisionAllow, RunID: runID, Workflow: workflowID, CommandID: commandID,
		Paths: paths, Categories: []string{"test-output"}, Hashes: hashes,
	}); err != nil {
		return nil, workflowCLIError(&workflows.WorkflowError{Code: workflows.FailurePrerequisiteMissing, Path: ".workbench/audit.jsonl", Message: "command completed but its result could not be audited"})
	}
	return output, nil
}

func projectByID(manifest *workspace.Workspace, id string) (workspace.Project, bool) {
	for _, project := range manifest.Projects {
		if project.ID == id {
			return project, true
		}
	}
	return workspace.Project{}, false
}

func auditDecision(outcome policy.Outcome) audit.Decision {
	switch outcome {
	case policy.Allow:
		return audit.DecisionAllow
	case policy.Ask:
		return audit.DecisionAsk
	default:
		return audit.DecisionDeny
	}
}

func contentHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func safeCommandOutput(content []byte) bool {
	if bytes.ContainsRune(content, 0) || !utf8.Valid(content) {
		return false
	}
	for _, value := range string(content) {
		if unicode.IsControl(value) && value != '\n' && value != '\r' && value != '\t' {
			return false
		}
	}
	return true
}

type boundedCapture struct {
	data      []byte
	truncated bool
}

func (capture *boundedCapture) Write(content []byte) (int, error) {
	remaining := maxCommandOutputBytes - len(capture.data)
	if remaining > 0 {
		if len(content) < remaining {
			remaining = len(content)
		}
		capture.data = append(capture.data, content[:remaining]...)
	}
	if remaining < len(content) {
		capture.truncated = true
	}
	return len(content), nil
}

func writeWorkflowRunText(writer io.Writer, output WorkflowRunOutput) {
	packet := output.Context
	fmt.Fprintf(writer, "Workflow: %s\nRun ID: %s\n", packet.WorkflowID, output.RunID)
	fmt.Fprintf(writer, "Instruction (%s):\n%s\n", packet.InstructionPath, packet.Instruction)
	if len(packet.Inputs) > 0 {
		fmt.Fprintln(writer, "Inputs:")
		for _, input := range packet.Inputs {
			fmt.Fprintf(writer, "- %s (sha256=%s, bytes=%d, lines=%d)\n%s\n", input.Path, input.SHA256, input.Bytes, input.LineCount, input.Content)
		}
	}
	if len(packet.Summaries) > 0 {
		fmt.Fprintln(writer, "Summaries:")
		for _, summary := range packet.Summaries {
			fmt.Fprintf(writer, "- %s [%s]: %s\n", summary.Path, summary.SummaryKind, summary.Content)
		}
	}
	fmt.Fprintf(writer, "Allowed skills: %s\nAllowed commands: %s\nRead paths: %s\nWrite paths: %s\n", strings.Join(packet.AllowedSkills, ", "), strings.Join(packet.AllowedCommands, ", "), strings.Join(packet.ReadPaths, ", "), strings.Join(packet.WritePaths, ", "))
	if len(packet.ExpectedOutputs) > 0 {
		fmt.Fprintln(writer, "Expected outputs:")
		for _, output := range packet.ExpectedOutputs {
			fmt.Fprintf(writer, "- %s (%s)\n", output.Path, output.Status)
		}
	}
	fmt.Fprintf(writer, "Next handoff: %s\n", packet.NextHandoff)
	for _, omission := range packet.Omissions {
		fmt.Fprintf(writer, "Omitted: %s (%s)\n", omission.Path, omission.Reason)
	}
	for _, warning := range packet.Warnings {
		fmt.Fprintf(writer, "Warning: %s\n", warning)
	}
	if output.Command != nil {
		fmt.Fprintf(writer, "Command %s exit code: %d\n", output.Command.ID, output.Command.ExitCode)
		if output.Command.Stdout != "" {
			fmt.Fprintf(writer, "stdout:\n%s\n", output.Command.Stdout)
		}
		if output.Command.Stderr != "" {
			fmt.Fprintf(writer, "stderr:\n%s\n", output.Command.Stderr)
		}
		if output.Command.OutputOmitted {
			fmt.Fprintln(writer, "Command output omitted by data policy.")
		}
		if output.Command.Truncated {
			fmt.Fprintln(writer, "Command output truncated at the context size limit.")
		}
	}
}

func writeWorkflowVerifyText(writer io.Writer, result workflows.WorkflowResult) {
	fmt.Fprintf(writer, "Workflow: %s\nRun ID: %s\nStatus: %s\n", result.WorkflowID, result.RunID, result.Status)
	for _, output := range result.Outputs {
		fmt.Fprintf(writer, "- %s (%s, sha256 before=%s after=%s)\n", output.Path, output.Status, output.HashBefore, output.HashAfter)
	}
	for _, path := range result.OutOfScopeChanges {
		fmt.Fprintf(writer, "Out-of-scope change: %s\n", path)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(writer, "Warning: %s\n", warning)
	}
}
