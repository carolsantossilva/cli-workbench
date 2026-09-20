package policy

import (
	"fmt"
	"path"
	"strings"

	"station/internal/workspace"
)

type Outcome string

const (
	Allow Outcome = "allow"
	Ask   Outcome = "ask"
	Deny  Outcome = "deny"
)

type ActionKind string

const (
	ActionRead          ActionKind = "read"
	ActionWrite         ActionKind = "write"
	ActionCommand       ActionKind = "command"
	ActionExternalWrite ActionKind = "external-write"
	ActionDataSend      ActionKind = "data-send"
)

type Action struct {
	Kind                 ActionKind
	ProjectID            string
	ExternalRepositoryID string
	Path                 string
	CommandID            string
	DataCategory         string
	Provider             string
}

type Decision struct {
	Outcome     Outcome `json:"decision"`
	Reason      string  `json:"reason"`
	MatchedRule string  `json:"matchedRule,omitempty"`
}

type executionRule struct {
	kind        ActionKind
	project     string
	path        string
	commandID   string
	category    string
	provider    string
	decision    Outcome
	index       int
	specificity int
}

// authorize evaluates lexical safety blocks, explicit execution rules,
// workflow allowlists, then the execution default. Callers must first use
// assertWorkspacePath against the relevant root to check physical containment;
// this function does not receive a filesystem root.
func authorize(action Action, manifest *workspace.Workspace, workflowID string) Decision {
	if manifest == nil {
		return decision(Deny, "workspace manifest is unavailable", "manifest")
	}
	if len(ValidateManifestPolicies(manifest)) > 0 {
		return decision(Deny, "workspace policy contract is invalid", "manifest-policies")
	}
	workflow, exists := manifest.Workflows[workflowID]
	if !exists {
		return decision(Deny, "workflow is not declared", "workflow")
	}
	if action.Kind == ActionExternalWrite {
		return decision(Deny, "external repository writes are not supported", "external-read-only")
	}
	if action.Kind == ActionDataSend {
		return decision(Deny, "Station does not send data to providers", "no-provider-send")
	}
	if action.Kind != ActionRead && action.Kind != ActionWrite && action.Kind != ActionCommand {
		return decision(Deny, "action kind is not supported", "invalid-action")
	}
	if action.DataCategory == "secret" || isEnvPath(action.Path) {
		return decision(Deny, "secret data is excluded", "secret-block")
	}
	if action.Kind == ActionRead || action.Kind == ActionWrite {
		if !validConcretePath(action.Path) {
			return decision(Deny, "path is outside the workspace contract", "path-boundary")
		}
		if isAuditPath(action.Path) {
			return decision(Deny, "Station audit data is not available to workflow operations", "audit-log-excluded")
		}
		if action.ProjectID != "" {
			project, found := projectByID(manifest, action.ProjectID)
			if !found {
				return decision(Deny, "project is not declared", "project")
			}
			if !pathWithinProject(project.Path, action.Path) {
				return decision(Deny, "path is outside the declared project", "project-path-boundary")
			}
		}
	}
	if action.Kind == ActionCommand {
		command, found := commandByID(manifest, action.CommandID)
		if !found {
			return decision(Deny, "command is not declared", "command")
		}
		if isShell(command.Executable, command.Args) {
			return decision(Deny, "shell executables are not allowed", "no-shell")
		}
		if _, found := projectByID(manifest, command.Project); !found {
			return decision(Deny, "command project is not declared", "command.project")
		}
	}

	rules, defaultOutcome, err := parseExecutionPolicy(manifest.Policies.Execution)
	if err != nil {
		return decision(Deny, "execution policy is invalid", "invalid-execution-policy")
	}
	if action.ExternalRepositoryID != "" {
		return authorizeExternalRead(action, manifest, workflow, rules)
	}
	if rule, ok := matchingRule(action, rules); ok {
		if rule.decision != Allow {
			return decision(rule.decision, "matched explicit execution rule", fmt.Sprintf("execution.rules[%d]", rule.index))
		}
		if !workflowAllows(action, workflow) {
			return workflowMissDecision(action, defaultOutcome)
		}
		return decision(Allow, "explicit execution rule and workflow scope allow the action", fmt.Sprintf("execution.rules[%d]", rule.index))
	}
	if workflowAllows(action, workflow) {
		return decision(Allow, "action is declared by the workflow allowlist", "workflow")
	}
	return workflowMissDecision(action, defaultOutcome)
}

func workflowAllows(action Action, workflow workspace.Workflow) bool {
	switch action.Kind {
	case ActionCommand:
		return contains(workflow.AllowedCommands, action.CommandID)
	case ActionRead:
		return matchesAnyPath(workflow.ReadPaths, action.Path)
	case ActionWrite:
		return matchesAnyPath(workflow.WritePaths, action.Path)
	default:
		return false
	}
}

func workflowMissDecision(action Action, defaultOutcome Outcome) Decision {
	if action.Kind == ActionWrite {
		return decision(Deny, "write path is outside the workflow allowlist", "workflow.writePaths")
	}
	if defaultOutcome != Allow {
		return decision(defaultOutcome, "no matching authorization rule", "execution.default")
	}
	switch action.Kind {
	case ActionCommand:
		return decision(Ask, "command is outside the workflow allowlist", "workflow.allowedCommands")
	default:
		return decision(Ask, "path is outside the workflow allowlist", "workflow.readPaths")
	}
}

// Authorize exposes policy evaluation to sibling internal packages. For read or
// write actions, callers must first call AssertWorkspacePath and then perform
// filesystem I/O through an os.Root handle, not the returned absolute path.
func Authorize(action Action, manifest *workspace.Workspace, workflowID string) Decision {
	return authorize(action, manifest, workflowID)
}

func ValidateManifestPolicies(manifest *workspace.Workspace) []workspace.Diagnostic {
	if manifest == nil {
		return []workspace.Diagnostic{{Code: "invalid", Message: "workspace manifest is required", Path: "/", Severity: "error"}}
	}
	var diagnostics []workspace.Diagnostic
	rules, _, err := parseExecutionPolicy(manifest.Policies.Execution)
	if err != nil {
		diagnostics = append(diagnostics, workspace.Diagnostic{Code: "invalid", Message: "execution policy is invalid", Path: "/policies/execution", Severity: "error"})
	} else {
		for _, rule := range rules {
			if rule.project != "" {
				if _, found := projectByID(manifest, rule.project); !found {
					diagnostics = append(diagnostics, workspace.Diagnostic{Code: "invalid", Message: "execution rule project does not exist", Path: fmt.Sprintf("/policies/execution/rules/%d/project", rule.index), Severity: "error"})
				}
			}
			if rule.commandID != "" {
				if _, found := commandByID(manifest, rule.commandID); !found {
					diagnostics = append(diagnostics, workspace.Diagnostic{Code: "invalid", Message: "execution rule command does not exist", Path: fmt.Sprintf("/policies/execution/rules/%d/commandId", rule.index), Severity: "error"})
				}
			}
		}
	}
	dataPolicy, err := ParseDataPolicy(manifest.Policies.Data)
	if err != nil {
		diagnostics = append(diagnostics, workspace.Diagnostic{Code: "invalid", Message: "data policy is invalid", Path: "/policies/data", Severity: "error"})
	} else {
		for _, rule := range dataPolicy.Rules {
			if rule.ProjectID != "" {
				if _, found := projectByID(manifest, rule.ProjectID); !found {
					diagnostics = append(diagnostics, workspace.Diagnostic{Code: "invalid", Message: "data rule project does not exist", Path: fmt.Sprintf("/policies/data/rules/%d/project", rule.Index), Severity: "error"})
				}
			}
		}
	}
	return diagnostics
}

func authorizeExternalRead(action Action, manifest *workspace.Workspace, workflow workspace.Workflow, rules []executionRule) Decision {
	if action.Kind != ActionRead {
		return decision(Deny, "external repositories are read-only", "external-read-only")
	}
	if !matchesAnyPath(workflow.ReadPaths, action.Path) {
		return decision(Deny, "path is not declared by the workflow read allowlist", "workflow.readPaths")
	}
	for _, repository := range manifest.ExternalRepositories {
		if repository.ID != action.ExternalRepositoryID {
			continue
		}
		if !repository.ReadOnly {
			return decision(Ask, "external repository is not explicitly read-only", "external-repository")
		}
		if matchesAnyPath(repository.AllowedReadPaths, action.Path) {
			if rule, ok := matchingRule(action, rules); ok {
				return decision(rule.decision, "matched explicit execution rule", fmt.Sprintf("execution.rules[%d]", rule.index))
			}
			return decision(Allow, "path is declared by the external repository read allowlist", "externalRepositories.allowedReadPaths")
		}
		return decision(Deny, "path is not allowed for the external repository", "external-read-only")
	}
	return decision(Deny, "external repository is not declared", "external-repository")
}

func parseExecutionPolicy(raw map[string]any) ([]executionRule, Outcome, error) {
	defaultOutcome := Ask
	if value, ok := raw["default"]; ok {
		parsed, valid := parseOutcome(value)
		if !valid {
			return nil, Deny, fmt.Errorf("invalid default decision")
		}
		defaultOutcome = parsed
	}
	value, hasRules := raw["rules"]
	if !hasRules {
		for key := range raw {
			if key != "default" {
				return nil, Deny, fmt.Errorf("unknown execution policy field")
			}
		}
		return nil, defaultOutcome, nil
	}
	rawRules, ok := value.([]any)
	if !ok {
		return nil, Deny, fmt.Errorf("execution rules must be an array")
	}
	for key := range raw {
		if key != "default" && key != "rules" {
			return nil, Deny, fmt.Errorf("unknown execution policy field")
		}
	}
	rules := make([]executionRule, 0, len(rawRules))
	for index, value := range rawRules {
		fields, ok := value.(map[string]any)
		if !ok {
			return nil, Deny, fmt.Errorf("execution rule must be an object")
		}
		rule, err := parseExecutionRule(fields, index)
		if err != nil {
			return nil, Deny, err
		}
		rules = append(rules, rule)
	}
	return rules, defaultOutcome, nil
}

func parseExecutionRule(fields map[string]any, index int) (executionRule, error) {
	rule := executionRule{index: index}
	for key, raw := range fields {
		value, ok := raw.(string)
		if !ok {
			return rule, fmt.Errorf("execution rule field must be a string")
		}
		switch key {
		case "kind":
			rule.kind = ActionKind(value)
		case "project":
			rule.project = value
		case "path":
			rule.path = value
		case "commandId":
			rule.commandID = value
		case "category":
			rule.category = value
		case "provider":
			rule.provider = value
		case "decision":
			parsed, valid := parseOutcome(value)
			if !valid {
				return rule, fmt.Errorf("execution rule has invalid decision")
			}
			rule.decision = parsed
		default:
			return rule, fmt.Errorf("unknown execution rule field")
		}
	}
	if rule.decision == "" || (rule.kind == "" && rule.project == "" && rule.path == "" && rule.commandID == "" && rule.category == "" && rule.provider == "") {
		return rule, fmt.Errorf("execution rule must include a matcher and decision")
	}
	if rule.kind != "" && rule.kind != ActionRead && rule.kind != ActionWrite && rule.kind != ActionCommand {
		return rule, fmt.Errorf("execution rule has unsupported action kind")
	}
	if rule.path != "" && !validPathPattern(rule.path) {
		return rule, fmt.Errorf("execution rule path is invalid")
	}
	for _, value := range []string{rule.project, rule.commandID, rule.category, rule.provider} {
		if strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
			return rule, fmt.Errorf("execution rule matcher is invalid")
		}
	}
	rule.specificity = boolCount(rule.kind != "", rule.project != "", rule.path != "", rule.commandID != "", rule.category != "", rule.provider != "")
	return rule, nil
}

func matchingRule(action Action, rules []executionRule) (executionRule, bool) {
	var best executionRule
	found := false
	for _, rule := range rules {
		if rule.kind != "" && rule.kind != action.Kind ||
			rule.project != "" && rule.project != action.ProjectID ||
			rule.commandID != "" && rule.commandID != action.CommandID ||
			rule.category != "" && rule.category != action.DataCategory ||
			rule.provider != "" && rule.provider != action.Provider ||
			rule.path != "" && !matchPathPattern(rule.path, action.Path) {
			continue
		}
		if !found || rule.specificity > best.specificity || (rule.specificity == best.specificity && restrictiveRank(rule.decision) > restrictiveRank(best.decision)) {
			best = rule
			found = true
		}
	}
	return best, found
}

func parseOutcome(value any) (Outcome, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	outcome := Outcome(text)
	return outcome, outcome == Allow || outcome == Ask || outcome == Deny
}

func restrictiveRank(outcome Outcome) int {
	switch outcome {
	case Deny:
		return 3
	case Ask:
		return 2
	case Allow:
		return 1
	default:
		return 0
	}
}

func decision(outcome Outcome, reason, matchedRule string) Decision {
	return Decision{Outcome: outcome, Reason: reason, MatchedRule: matchedRule}
}

func commandByID(manifest *workspace.Workspace, id string) (workspace.Command, bool) {
	var found workspace.Command
	count := 0
	for _, command := range manifest.Commands {
		if command.ID == id {
			found = command
			count++
		}
	}
	return found, count == 1
}

func projectByID(manifest *workspace.Workspace, id string) (workspace.Project, bool) {
	var found workspace.Project
	count := 0
	for _, project := range manifest.Projects {
		if project.ID == id {
			found = project
			count++
		}
	}
	return found, count == 1
}

func pathWithinProject(projectPath, actionPath string) bool {
	return actionPath == projectPath || strings.HasPrefix(actionPath, strings.TrimSuffix(projectPath, "/")+"/")
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func isEnvPath(value string) bool {
	base := path.Base(strings.ReplaceAll(value, "\\", "/"))
	return strings.HasPrefix(base, ".env")
}

func isAuditPath(value string) bool {
	clean := path.Clean(value)
	return clean == ".station" || strings.HasPrefix(clean, ".station/")
}
