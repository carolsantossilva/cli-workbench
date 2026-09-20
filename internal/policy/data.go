package policy

import (
	"errors"
	"strconv"
	"strings"
)

type DataPolicy struct {
	Default           Outcome
	CategoryDecisions map[string]Outcome
	Providers         []string
	ProjectID         string // Set by the caller for the project containing the current path.
	Rules             []DataRule
}

type DataRule struct {
	ProjectID   string
	Path        string
	Category    string
	Decision    Outcome
	Index       int
	Specificity int
}

type SecretFinding struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	SHA256 string `json:"sha256"`
}

type ScanResult struct {
	Decision Decision        `json:"decision"`
	Findings []SecretFinding `json:"findings,omitempty"`
}

func scanData(path, content, category string, policy DataPolicy) ScanResult {
	if isEnvPath(path) {
		return ScanResult{
			Decision: decision(Deny, "environment files are excluded from context", "secret-block"),
			Findings: []SecretFinding{{Type: "env-file", Path: path, Line: 1, SHA256: hashContent(content)}},
		}
	}
	if category == "secret" {
		return ScanResult{
			Decision: decision(Deny, "secret data is excluded from context", "secret-block"),
			Findings: []SecretFinding{{Type: "secret-category", Path: path, Line: 1, SHA256: hashContent(content)}},
		}
	}
	findings := detectPEM(path, content)
	if len(findings) > 0 {
		return ScanResult{Decision: decision(Deny, "PEM material is excluded from context", "secret-block"), Findings: findings}
	}
	if !knownDataCategory(category) {
		if rule, ok := matchingDataRule(path, category, policy); ok {
			ruleID := "data.rules." + strconv.Itoa(rule.Index)
			switch rule.Decision {
			case Deny:
				return ScanResult{Decision: decision(Deny, "matched explicit deny for unknown data", ruleID)}
			case Ask:
				return ScanResult{Decision: decision(Ask, "matched explicit ask for unknown data", ruleID)}
			default:
				return ScanResult{Decision: decision(Ask, "unknown data must be reviewed and cannot be allowed", ruleID)}
			}
		}
		if outcome, exists := policy.CategoryDecisions[category]; exists {
			switch outcome {
			case Deny:
				return ScanResult{Decision: decision(Deny, "matched unknown data category deny", "data.categories."+category)}
			case Ask:
				return ScanResult{Decision: decision(Ask, "matched unknown data category review", "data.categories."+category)}
			default:
				return ScanResult{Decision: decision(Ask, "unknown data must be reviewed and cannot be allowed", "data.categories."+category)}
			}
		}
		return ScanResult{Decision: decision(Ask, "data category is unknown", "data.default")}
	}
	if rule, ok := matchingDataRule(path, category, policy); ok {
		return ScanResult{Decision: decision(rule.Decision, "matched project/path/category data rule", "data.rules."+strconv.Itoa(rule.Index))}
	}
	if outcome, exists := policy.CategoryDecisions[category]; exists {
		if validOutcome(outcome) {
			return ScanResult{Decision: decision(outcome, "matched data category policy", "data.categories."+category)}
		}
		return ScanResult{Decision: decision(Ask, "data category policy is invalid", "invalid-data-policy")}
	}
	if validOutcome(policy.Default) {
		return ScanResult{Decision: decision(policy.Default, "matched data policy default", "data.default")}
	}
	return ScanResult{Decision: decision(Ask, "no data policy rule matched", "data.default")}
}

func ScanData(path, content, category string, policy DataPolicy) ScanResult {
	return scanData(path, content, category, policy)
}

// ParseDataPolicy reads the version-1 data policy fields without enabling any
// provider integration. Unknown fields and invalid decisions fail closed.
func ParseDataPolicy(raw map[string]any) (DataPolicy, error) {
	policy := DataPolicy{Default: Ask, CategoryDecisions: make(map[string]Outcome)}
	for key, value := range raw {
		switch key {
		case "default":
			outcome, ok := parseOutcome(value)
			if !ok {
				return DataPolicy{}, errInvalidDataPolicy
			}
			policy.Default = outcome
		case "categories":
			categories, ok := value.(map[string]any)
			if !ok {
				return DataPolicy{}, errInvalidDataPolicy
			}
			for category, rawDecision := range categories {
				if category != "source" && category != "design" && category != "test-output" && category != "secret" && category != "unknown" {
					return DataPolicy{}, errInvalidDataPolicy
				}
				outcome, ok := parseOutcome(rawDecision)
				if !ok || category == "secret" && outcome != Deny || category == "unknown" && outcome == Allow {
					return DataPolicy{}, errInvalidDataPolicy
				}
				policy.CategoryDecisions[category] = outcome
			}
		case "providers":
			providers, ok := value.([]any)
			if !ok {
				return DataPolicy{}, errInvalidDataPolicy
			}
			seen := make(map[string]struct{}, len(providers))
			for _, rawProvider := range providers {
				provider, ok := rawProvider.(string)
				if !ok || provider == "" || strings.TrimSpace(provider) != provider || strings.ContainsAny(provider, "\r\n\x00") {
					return DataPolicy{}, errInvalidDataPolicy
				}
				if _, duplicate := seen[provider]; duplicate {
					return DataPolicy{}, errInvalidDataPolicy
				}
				seen[provider] = struct{}{}
				policy.Providers = append(policy.Providers, provider)
			}
		case "rules":
			rawRules, ok := value.([]any)
			if !ok {
				return DataPolicy{}, errInvalidDataPolicy
			}
			for index, rawRule := range rawRules {
				fields, ok := rawRule.(map[string]any)
				if !ok {
					return DataPolicy{}, errInvalidDataPolicy
				}
				rule, ok := parseDataRule(fields, index)
				if !ok {
					return DataPolicy{}, errInvalidDataPolicy
				}
				policy.Rules = append(policy.Rules, rule)
			}
		default:
			return DataPolicy{}, errInvalidDataPolicy
		}
	}
	return policy, nil
}

func parseDataRule(fields map[string]any, index int) (DataRule, bool) {
	rule := DataRule{Index: index}
	for key, raw := range fields {
		value, ok := raw.(string)
		if !ok {
			return rule, false
		}
		switch key {
		case "project":
			rule.ProjectID = value
		case "path":
			rule.Path = value
		case "category":
			rule.Category = value
		case "decision":
			outcome, valid := parseOutcome(value)
			if !valid {
				return rule, false
			}
			rule.Decision = outcome
		default:
			return rule, false
		}
	}
	if rule.Decision == "" || (rule.ProjectID == "" && rule.Path == "" && rule.Category == "") {
		return rule, false
	}
	if rule.ProjectID != "" && (strings.TrimSpace(rule.ProjectID) != rule.ProjectID || strings.ContainsAny(rule.ProjectID, "\r\n\x00")) {
		return rule, false
	}
	if rule.Path != "" && !validPathPattern(rule.Path) {
		return rule, false
	}
	if rule.Category != "" && rule.Category != "source" && rule.Category != "design" && rule.Category != "test-output" && rule.Category != "secret" && rule.Category != "unknown" {
		return rule, false
	}
	if rule.Category == "secret" && rule.Decision != Deny || rule.Category == "unknown" && rule.Decision == Allow {
		return rule, false
	}
	rule.Specificity = boolCount(rule.ProjectID != "", rule.Path != "", rule.Category != "")
	return rule, true
}

func matchingDataRule(filePath, category string, policy DataPolicy) (DataRule, bool) {
	var best DataRule
	found := false
	for _, rule := range policy.Rules {
		if rule.ProjectID != "" && rule.ProjectID != policy.ProjectID ||
			rule.Path != "" && !matchPathPattern(rule.Path, filePath) ||
			rule.Category != "" && rule.Category != category {
			continue
		}
		if !found || rule.Specificity > best.Specificity || (rule.Specificity == best.Specificity && restrictiveRank(rule.Decision) > restrictiveRank(best.Decision)) {
			best = rule
			found = true
		}
	}
	return best, found
}

func knownDataCategory(category string) bool {
	switch category {
	case "source", "design", "test-output", "secret":
		return true
	default:
		return false
	}
}

var errInvalidDataPolicy = errors.New("data policy is invalid")

func validOutcome(outcome Outcome) bool {
	return outcome == Allow || outcome == Ask || outcome == Deny
}
