package cli

import (
	"fmt"

	"workbench/internal/workspace"
)

type ErrorCode string

const (
	CodeInvalid             ErrorCode = "invalid"
	CodeAsk                 ErrorCode = "ask"
	CodeDeny                ErrorCode = "deny"
	CodeArtifactMissing     ErrorCode = "artifact-missing"
	CodePrerequisiteMissing ErrorCode = "prerequisite-missing"
)

type Error struct {
	Code        ErrorCode
	Status      string
	Message     string
	Diagnostics []workspace.Diagnostic
}

func (err *Error) Error() string { return err.Message }

func exitCode(code ErrorCode) int {
	switch code {
	case CodeAsk:
		return 3
	case CodeDeny:
		return 4
	case CodeArtifactMissing, CodePrerequisiteMissing:
		return 5
	default:
		return 2
	}
}

func invalidError(format string, args ...any) error {
	return &Error{Code: CodeInvalid, Status: "invalid", Message: fmt.Sprintf(format, args...)}
}
