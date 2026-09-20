package scaffold

import (
	"os"
	"testing"
)

func TestValidateModulePathLexicalRules(t *testing.T) {
	tests := []struct {
		path  string
		valid bool
	}{
		{path: "example/server", valid: true},
		{path: "github.com/org/server", valid: true},
		{path: "-local/server", valid: true},
		{path: "server~beta", valid: true},
		{path: "", valid: true},
		{path: "foo:bar", valid: false},
		{path: "example//server", valid: false},
		{path: "example/./server", valid: false},
		{path: "example/../server", valid: false},
		{path: "example/.hidden", valid: false},
		{path: "example/server.", valid: false},
		{path: "example/org?repo", valid: false},
		{path: "example/café", valid: false},
		{path: "example/CON", valid: false},
		{path: "example/com1", valid: false},
		{path: "example/aux.txt", valid: false},
		{path: "example/server~1", valid: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			err := validateModulePath(test.path)
			if (err == nil) != test.valid {
				t.Fatalf("validateModulePath(%q) error = %v, valid = %t", test.path, err, test.valid)
			}
		})
	}
}

func TestScaffoldRejectsInvalidModulePathBeforeWriting(t *testing.T) {
	root := t.TempDir()
	_, err := ScaffoldMonorepo(root, Design{ModulePath: "foo:bar"}, Policy{})
	if err == nil {
		t.Fatal("ScaffoldMonorepo accepted invalid Go module path foo:bar")
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid module path left scaffold files behind: %v", entries)
	}
}
