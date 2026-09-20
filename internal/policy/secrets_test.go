package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestScanDataOmitsEnvFilesAndDoesNotReturnSecretContent(t *testing.T) {
	secret := "SERVICE_PASSWORD=very-private-value"
	result := scanData(".env.local", secret, "source", DataPolicy{})
	if result.Decision.Outcome != Deny {
		t.Fatalf("decision = %q, want deny", result.Decision.Outcome)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "very-private-value") {
		t.Fatalf("scan result leaked secret: %s", encoded)
	}
	if len(result.Findings) != 1 || result.Findings[0].Type != "env-file" || result.Findings[0].Line != 1 || result.Findings[0].SHA256 == "" {
		t.Fatalf("findings = %#v, want redacted env-file metadata", result.Findings)
	}
}

func TestScanDataDetectsPEMAndReturnsOnlyRedactedMetadata(t *testing.T) {
	secret := "-----BEGIN PRIVATE KEY-----\nvery-private-pem-material\n-----END PRIVATE KEY-----\n"
	result := scanData("apps/server/key.pem", secret, "source", DataPolicy{})
	if result.Decision.Outcome != Deny {
		t.Fatalf("decision = %q, want deny", result.Decision.Outcome)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "very-private-pem-material") || strings.Contains(string(encoded), "BEGIN PRIVATE KEY") {
		t.Fatalf("PEM result leaked secret content: %s", encoded)
	}
	if len(result.Findings) != 1 || result.Findings[0].Type != "pem" || result.Findings[0].Line != 1 || result.Findings[0].SHA256 == "" {
		t.Fatalf("findings = %#v, want redacted PEM metadata", result.Findings)
	}
}

func TestScanDataUsesAskForUnknownCategory(t *testing.T) {
	result := scanData("docs/notes.md", "ordinary text", "unknown", DataPolicy{})
	if result.Decision.Outcome != Ask {
		t.Fatalf("unknown data category = %q, want ask", result.Decision.Outcome)
	}
}

func TestScanDataDoesNotAllowUnrecognizedCategoryFromUnparsedPolicy(t *testing.T) {
	result := ScanData("docs/notes.md", "ordinary text", "accounting", DataPolicy{
		Rules: []DataRule{{Category: "accounting", Decision: Allow}},
	})
	if result.Decision.Outcome != Ask {
		t.Fatalf("unrecognized category with unparsed allow rule = %q, want ask", result.Decision.Outcome)
	}
}

func TestScanDataPreservesMatchedUnknownCategoryRule(t *testing.T) {
	policy, err := ParseDataPolicy(map[string]any{
		"rules": []any{map[string]any{"category": "unknown", "decision": "ask"}},
	})
	if err != nil {
		t.Fatalf("ParseDataPolicy() error = %v", err)
	}
	result := ScanData("docs/notes.md", "ordinary text", "unknown", policy)
	if result.Decision.Outcome != Ask || result.Decision.MatchedRule != "data.rules.0" {
		t.Fatalf("decision = %#v, want ask from the explicit unknown-category rule", result.Decision)
	}
}

func TestParseDataPolicyValidatesKnownCategoriesAndProviderList(t *testing.T) {
	policy, err := ParseDataPolicy(map[string]any{
		"default":    "ask",
		"categories": map[string]any{"source": "allow", "secret": "deny", "unknown": "ask"},
		"providers":  []any{},
	})
	if err != nil {
		t.Fatalf("ParseDataPolicy() error = %v", err)
	}
	if result := scanData("apps/web/src/App.tsx", "source", "source", policy); result.Decision.Outcome != Allow {
		t.Fatalf("source result = %q, want allow", result.Decision.Outcome)
	}
	if result := scanData(".env.local", "PASSWORD=x", "source", policy); result.Decision.Outcome != Deny {
		t.Fatalf("secret result = %q, want deny", result.Decision.Outcome)
	}
}

func TestParseDataPolicyAllowsExplicitUnknownDeny(t *testing.T) {
	policy, err := ParseDataPolicy(map[string]any{
		"categories": map[string]any{"unknown": "deny"},
	})
	if err != nil {
		t.Fatalf("ParseDataPolicy() error = %v", err)
	}
	result := ScanData("docs/notes.md", "ordinary text", "unknown", policy)
	if result.Decision.Outcome != Deny || result.Decision.MatchedRule != "data.categories.unknown" {
		t.Fatalf("decision = %#v, want explicit category deny", result.Decision)
	}
}

func TestParseDataPolicyRejectsSecretAllowUnknownAllowAndUnknownFields(t *testing.T) {
	for name, value := range map[string]map[string]any{
		"secret allow":  {"categories": map[string]any{"secret": "allow"}},
		"unknown allow": {"categories": map[string]any{"unknown": "allow"}},
		"unknown field": {"providerList": []any{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDataPolicy(value); err == nil {
				t.Fatalf("ParseDataPolicy(%#v) succeeded", value)
			}
		})
	}
}

func TestScanDataAppliesMostSpecificProjectPathCategoryRule(t *testing.T) {
	policy, err := ParseDataPolicy(map[string]any{
		"default":    "deny",
		"categories": map[string]any{"source": "allow"},
		"rules": []any{
			map[string]any{"path": "apps/web/**", "category": "source", "decision": "allow"},
			map[string]any{"project": "web", "path": "apps/web/private/**", "category": "source", "decision": "deny"},
		},
	})
	if err != nil {
		t.Fatalf("ParseDataPolicy() error = %v", err)
	}
	policy.ProjectID = "web"
	result := scanData("apps/web/private/config.ts", "ordinary source", "source", policy)
	if result.Decision.Outcome != Deny || result.Decision.MatchedRule != "data.rules.1" {
		t.Fatalf("data decision = %#v, want most-specific deny rule", result.Decision)
	}
}
