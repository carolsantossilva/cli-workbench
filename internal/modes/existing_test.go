package modes

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInspectWithoutManifestAcceptsEmptyRoot(t *testing.T) {
	root := t.TempDir()
	handle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	inspection, err := inspectWithoutManifest(root, handle)
	if err != nil {
		t.Fatalf("inspect empty root: %v", err)
	}
	if inspection.ManifestStatus != "absent" || len(inspection.Entries) != 0 {
		t.Fatalf("inspection = %#v, want absent manifest and no entries", inspection)
	}
}

func TestInspectWithoutManifestRejectsEntryLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i <= maxInspectionEntries; i++ {
		name := filepath.Join(root, fmt.Sprintf("entry-%05d", i))
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatalf("create root entry %d: %v", i, err)
		}
	}

	handle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	_, err = inspectWithoutManifest(root, handle)
	if err == nil || !strings.Contains(err.Error(), "immediate-root entry limit") {
		t.Fatalf("inspectWithoutManifest error = %v, want immediate-root entry limit", err)
	}
}

func TestInspectDeclaredPathsRejectsVisitLimit(t *testing.T) {
	root := t.TempDir()
	for parent := 0; parent < 98; parent++ {
		for child := 0; child < 100; child++ {
			directory := filepath.Join(root, "docs", fmt.Sprintf("parent-%03d", parent), fmt.Sprintf("child-%03d", child))
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatalf("create declared-path fixture %d/%d: %v", parent, child, err)
			}
		}
	}
	for link := 0; link < 103; link++ {
		name := filepath.Join(root, "docs", fmt.Sprintf("link-%03d", link))
		if err := os.Symlink("missing-target", name); err != nil {
			t.Skipf("symlink fixture unavailable: %v", err)
		}
	}

	handle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	_, err = inspectDeclaredPaths(handle, []string{"docs/**/not-present.md"})
	if err == nil || !strings.Contains(err.Error(), "declared-path entry limit") {
		t.Fatalf("inspectDeclaredPaths error = %v, want declared-path entry limit", err)
	}
}

func TestInspectDeclaredPathsPrunesImpossibleDescendant(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "docs", "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "visible.md"), []byte("visible"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "hidden.md"), []byte("outside pattern depth"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(nested, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(nested, 0o700)
	probe, err := os.Open(nested)
	if err == nil {
		_ = probe.Close()
		t.Skip("filesystem permissions do not prevent directory reads")
	}

	handle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	entries, err := inspectDeclaredPaths(handle, []string{"docs/*.md"})
	if err != nil {
		t.Fatalf("inspection descended into an impossible pattern branch: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "docs/visible.md" {
		t.Fatalf("inspection entries = %#v, want only docs/visible.md", entries)
	}
}

func TestInspectDeclaredPathsReturnsOnlyDeclaredPatternMatches(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		files   []string
		want    []string
	}{
		{
			name:    "single-level wildcard",
			pattern: "docs/*.md",
			files:   []string{"docs/direct.md", "docs/nested/child.md"},
			want:    []string{"docs/direct.md"},
		},
		{
			name:    "one directory wildcard",
			pattern: "docs/*/file.md",
			files:   []string{"docs/group/file.md", "docs/group/deep/file.md"},
			want:    []string{"docs/group/file.md"},
		},
		{
			name:    "recursive wildcard",
			pattern: "docs/**/file.md",
			files:   []string{"docs/file.md", "docs/group/file.md", "docs/group/deep/file.md"},
			want:    []string{"docs/file.md", "docs/group/deep/file.md", "docs/group/file.md"},
		},
		{
			name:    "root recursive wildcard",
			pattern: "**/README.md",
			files:   []string{"README.md", "docs/README.md", "apps/web/README.md"},
			want:    []string{"README.md", "apps/web/README.md", "docs/README.md"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range test.files {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			handle, err := os.OpenRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Close()
			entries, err := inspectDeclaredPaths(handle, []string{test.pattern})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(entries))
			for i, entry := range entries {
				got[i] = entry.Path
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("inspection paths = %#v, want %#v", got, test.want)
			}
		})
	}
}
