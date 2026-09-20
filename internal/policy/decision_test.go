package policy

import (
	"testing"

	"station/internal/workspace"
)

func TestAuthorizeUsesMostSpecificRuleAndBlocksSecrets(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{
		"default": "ask",
		"rules": []any{
			map[string]any{"kind": "write", "project": "web", "path": "apps/web/**", "decision": "allow"},
			map[string]any{"kind": "write", "project": "web", "path": "apps/web/.env*", "decision": "allow"},
		},
	}

	decision := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "apps/web/.env.local", DataCategory: "secret"}, manifest, "build")
	if decision.Outcome != Deny {
		t.Fatalf("secret decision = %q, want deny", decision.Outcome)
	}
	if decision.MatchedRule != "secret-block" {
		t.Fatalf("matched rule = %q, want secret-block", decision.MatchedRule)
	}
}

func TestAuthorizeAppliesSpecificRuleBeforeWorkflowAllowlist(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{
		"default": "ask",
		"rules": []any{
			map[string]any{"kind": "write", "path": "apps/**", "decision": "allow"},
			map[string]any{"kind": "write", "project": "web", "path": "apps/web/src/**", "decision": "deny"},
		},
	}

	decision := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "apps/web/src/App.tsx"}, manifest, "build")
	if decision.Outcome != Deny {
		t.Fatalf("decision = %q, want deny", decision.Outcome)
	}
	if decision.MatchedRule != "execution.rules[1]" {
		t.Fatalf("matched rule = %q, want execution.rules[1]", decision.MatchedRule)
	}
}

func TestAuthorizeAllowsOnlyWorkflowDeclaredWritePaths(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{"default": "deny"}

	allowed := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "apps/web/src/App.tsx"}, manifest, "build")
	if allowed.Outcome != Allow {
		t.Fatalf("workflow-declared path = %q, want allow", allowed.Outcome)
	}

	unlisted := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "private/notes.md"}, manifest, "build")
	if unlisted.Outcome != Deny {
		t.Fatalf("unlisted write = %q, want deny from default", unlisted.Outcome)
	}
}

func TestAuthorizeUsesAskForUnmatchedReadAndDenyForExternalWrite(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{"default": "ask"}

	read := authorize(Action{Kind: ActionRead, Path: "private/unknown.txt"}, manifest, "build")
	if read.Outcome != Ask {
		t.Fatalf("unmatched read = %q, want ask", read.Outcome)
	}

	write := authorize(Action{Kind: ActionExternalWrite, Path: "../reference"}, manifest, "build")
	if write.Outcome != Deny {
		t.Fatalf("external write = %q, want deny", write.Outcome)
	}
}

func TestAuthorizeRejectsMalformedPolicyWithoutAllowing(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{
		"default": "allow",
		"rules":   []any{map[string]any{"kind": "write", "path": "apps/web/**", "decision": "maybe"}},
	}

	decision := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "apps/web/src/App.tsx"}, manifest, "build")
	if decision.Outcome == Allow {
		t.Fatalf("malformed policy unexpectedly allowed action: %#v", decision)
	}
}

func TestAuthorizeRejectsPolicyRulesWithUnknownProjectReferences(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{
		"rules": []any{map[string]any{"kind": "write", "project": "missing", "decision": "deny"}},
	}
	decision := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "apps/web/src/App.tsx"}, manifest, "build")
	if decision.Outcome != Deny || decision.MatchedRule != "manifest-policies" {
		t.Fatalf("decision = %#v, want fail-closed invalid policy", decision)
	}
}

func TestAuthorizeCannotUseBroadCommandRuleToAllowUndeclaredCommand(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{
		"rules": []any{map[string]any{"kind": "command", "decision": "allow"}},
	}
	decision := authorize(Action{Kind: ActionCommand, CommandID: "missing"}, manifest, "build")
	if decision.Outcome != Deny {
		t.Fatalf("undeclared command with broad rule = %q, want deny", decision.Outcome)
	}
}

func TestSpecificPolicyAllowCannotExpandWorkflowScope(t *testing.T) {
	manifest := policyFixture()
	workflow := manifest.Workflows["build"]
	workflow.WritePaths = []string{"apps/web/src/**"}
	workflow.AllowedCommands = nil
	manifest.Workflows["build"] = workflow
	manifest.Policies.Execution = map[string]any{
		"rules": []any{
			map[string]any{"kind": "write", "project": "web", "path": "apps/web/private/**", "decision": "allow"},
			map[string]any{"kind": "command", "commandId": "web.test", "decision": "allow"},
		},
	}

	write := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "apps/web/private/secret.md"}, manifest, "build")
	if write.Outcome != Deny {
		t.Fatalf("write beyond workflow scope = %q, want deny", write.Outcome)
	}
	command, _ := authorizeCommand("web.test", "build", manifest)
	if command.Outcome != Ask {
		t.Fatalf("command beyond workflow allowlist = %q, want ask", command.Outcome)
	}
}

func TestAllowDefaultCannotExpandWorkflowScope(t *testing.T) {
	manifest := policyFixture()
	manifest.Policies.Execution = map[string]any{"default": "allow"}
	workflow := manifest.Workflows["build"]
	workflow.WritePaths = []string{"apps/web/src/**"}
	manifest.Workflows["build"] = workflow

	decision := authorize(Action{Kind: ActionWrite, Path: "apps/web/private/file.txt"}, manifest, "build")
	if decision.Outcome != Deny {
		t.Fatalf("default allow beyond writePaths = %q, want deny", decision.Outcome)
	}
}

func TestAuthorizeRejectsOutsideWorkspacePath(t *testing.T) {
	manifest := policyFixture()
	decision := authorize(Action{Kind: ActionRead, ProjectID: "web", Path: "../outside.txt"}, manifest, "build")
	if decision.Outcome != Deny {
		t.Fatalf("traversal decision = %q, want deny", decision.Outcome)
	}
}

func TestAuthorizeRejectsUnknownProjectAndPathOutsideProject(t *testing.T) {
	manifest := policyFixture()
	unknown := authorize(Action{Kind: ActionWrite, ProjectID: "missing", Path: "apps/web/src/App.tsx"}, manifest, "build")
	if unknown.Outcome != Deny {
		t.Fatalf("unknown project = %q, want deny", unknown.Outcome)
	}
	outside := authorize(Action{Kind: ActionWrite, ProjectID: "web", Path: "docs/notes.md"}, manifest, "build")
	if outside.Outcome != Deny {
		t.Fatalf("path outside project = %q, want deny", outside.Outcome)
	}
}

func TestAuthorizeExcludesStationAuditTree(t *testing.T) {
	manifest := policyFixture()
	decision := authorize(Action{Kind: ActionRead, Path: ".station/audit.jsonl"}, manifest, "build")
	if decision.Outcome != Deny {
		t.Fatalf("audit read = %q, want deny", decision.Outcome)
	}
}

func TestAuthorizeAllowsOnlyDeclaredExternalReads(t *testing.T) {
	manifest := policyFixture()
	allowed := authorize(Action{Kind: ActionRead, ExternalRepositoryID: "reference", Path: "docs/guide.md"}, manifest, "build")
	if allowed.Outcome != Allow {
		t.Fatalf("declared external read = %q, want allow", allowed.Outcome)
	}
	denied := authorize(Action{Kind: ActionRead, ExternalRepositoryID: "reference", Path: "src/main.go"}, manifest, "build")
	if denied.Outcome != Deny {
		t.Fatalf("undeclared external read = %q, want deny", denied.Outcome)
	}
	workflow := manifest.Workflows["build"]
	workflow.ReadPaths = []string{"apps/web/**"}
	manifest.Workflows["build"] = workflow
	denied = authorize(Action{Kind: ActionRead, ExternalRepositoryID: "reference", Path: "docs/guide.md"}, manifest, "build")
	if denied.Outcome != Deny {
		t.Fatalf("external read outside workflow paths = %q, want deny", denied.Outcome)
	}
}

func policyFixture() *workspace.Workspace {
	return &workspace.Workspace{
		Workspace: workspace.Identity{ID: "fixture", Kind: "monorepo"},
		Projects:  []workspace.Project{{ID: "web", Path: "apps/web", Kind: "web", Language: "typescript"}},
		Commands:  []workspace.Command{{ID: "web.test", Category: "test", Executable: "npm", Args: []string{"test"}, Project: "web", TimeoutSeconds: 30}},
		Policies:  workspace.Policies{Execution: map[string]any{}, Data: map[string]any{}, ExternalChanges: map[string]any{}},
		Workflows: map[string]workspace.Workflow{
			"build": {AllowedCommands: []string{"web.test"}, ReadPaths: []string{"apps/web/**", "docs/**"}, WritePaths: []string{"apps/web/**"}},
		},
		ExternalRepositories: []workspace.ExternalRepository{{ID: "reference", Path: "../reference", ReadOnly: true, AllowedReadPaths: []string{"docs/**"}}},
	}
}
