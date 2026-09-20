package policy

import "testing"

func TestAuthorizeCommandReturnsDeclaredArgvWithoutShellParsing(t *testing.T) {
	manifest := policyFixture()
	manifest.Commands[0].Args = []string{"test", "--", "$(touch should-not-run);"}

	decision, command := authorizeCommand("web.test", "build", manifest)
	if decision.Outcome != Allow {
		t.Fatalf("decision = %q, want allow", decision.Outcome)
	}
	if len(command.Args) != 3 || command.Args[2] != "$(touch should-not-run);" {
		t.Fatalf("argv = %#v, want shell-looking text preserved as one argument", command.Args)
	}
}

func TestAuthorizeCommandRequiresWorkflowAllowlist(t *testing.T) {
	manifest := policyFixture()
	workflow := manifest.Workflows["review"]
	workflow.AllowedCommands = nil
	manifest.Workflows["review"] = workflow

	decision, _ := authorizeCommand("web.test", "review", manifest)
	if decision.Outcome != Ask {
		t.Fatalf("declared but unapproved command = %q, want ask", decision.Outcome)
	}
}

func TestAuthorizeCommandDeniesUnknownAndShellExecutable(t *testing.T) {
	manifest := policyFixture()
	if decision, _ := authorizeCommand("missing", "build", manifest); decision.Outcome != Deny {
		t.Fatalf("unknown command = %q, want deny", decision.Outcome)
	}

	manifest.Commands[0].Executable = "sh"
	decision, _ := authorizeCommand("web.test", "build", manifest)
	if decision.Outcome != Deny {
		t.Fatalf("shell executable decision = %q, want deny", decision.Outcome)
	}
}
