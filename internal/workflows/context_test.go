package workflows

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotWorkspaceCapturesDeclaredFileMetadata(t *testing.T) {
	root := t.TempDir()
	content := []byte("approved design input\n")
	writeTestFile(t, root, "docs/station/brief.md", content)

	snapshot, err := snapshotWorkspace(root, []string{"docs/station/brief.md"})
	if err != nil {
		t.Fatalf("snapshotWorkspace() error = %v", err)
	}
	if snapshot.Source != "filesystem" {
		t.Fatalf("snapshot source = %q, want filesystem", snapshot.Source)
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("snapshot entries = %#v, want one entry", snapshot.Entries)
	}
	entry := snapshot.Entries[0]
	wantHash := sha256.Sum256(content)
	if entry.Path != "docs/station/brief.md" || !entry.Exists || entry.SHA256 != hex.EncodeToString(wantHash[:]) || entry.Bytes != int64(len(content)) || entry.ModifiedAt.IsZero() {
		t.Fatalf("snapshot entry = %#v, want path, content hash, size and mtime", entry)
	}
}

func TestSnapshotWorkspaceRepresentsMissingExactPath(t *testing.T) {
	snapshot, err := snapshotWorkspace(t.TempDir(), []string{"docs/station/brief.md"})
	if err != nil {
		t.Fatalf("snapshotWorkspace() error = %v", err)
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("snapshot entries = %#v, want one missing entry", snapshot.Entries)
	}
	entry := snapshot.Entries[0]
	if entry.Path != "docs/station/brief.md" || entry.Exists || entry.SHA256 != "" || entry.Bytes != 0 {
		t.Fatalf("missing entry = %#v, want explicit absence", entry)
	}
}

func TestSnapshotWorkspaceExpandsOnlyMatchingPattern(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "docs/station/brief.md", []byte("brief"))
	writeTestFile(t, root, "apps/web/src/App.tsx", []byte("source"))

	snapshot, err := snapshotWorkspace(root, []string{"docs/**"})
	if err != nil {
		t.Fatalf("snapshotWorkspace() error = %v", err)
	}
	if len(snapshot.Entries) != 1 || snapshot.Entries[0].Path != "docs/station/brief.md" {
		t.Fatalf("snapshot entries = %#v, want only docs/station/brief.md", snapshot.Entries)
	}
}

func TestSnapshotWorkspaceExcludesSecretsCachesAndAudit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "docs/station/brief.md", []byte("safe\n"))
	writeTestFile(t, root, "docs/station/.env.local", []byte("PASSWORD=fixture-private-value"))
	writeTestFile(t, root, "docs/station/key.pem", []byte("-----BEGIN PRIVATE KEY-----\nfixture-private-value\n-----END PRIVATE KEY-----\n"))
	writeTestFile(t, root, "docs/station/node_modules/pkg/index.ts", []byte("module contents"))
	writeTestFile(t, root, "docs/station/.cache/context.json", []byte("cache contents"))
	writeTestFile(t, root, ".station/audit.jsonl", []byte("prior audit data"))

	snapshot, err := snapshotWorkspace(root, []string{"**"})
	if err != nil {
		t.Fatalf("snapshotWorkspace() error = %v", err)
	}
	got := make(map[string]bool, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		got[entry.Path] = true
	}
	for _, excluded := range []string{
		"docs/station/.env.local",
		"docs/station/key.pem",
		"docs/station/node_modules/pkg/index.ts",
		"docs/station/.cache/context.json",
		".station/audit.jsonl",
	} {
		if got[excluded] {
			t.Errorf("snapshot includes excluded path %q", excluded)
		}
	}
	if !got["docs/station/brief.md"] {
		t.Fatalf("snapshot omits the safe declared file: %#v", snapshot.Entries)
	}
}

func TestSnapshotWorkspaceRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "private.txt", []byte("outside"))
	if err := os.Symlink(outside, filepath.Join(root, "docs")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := snapshotWorkspace(root, []string{"docs/private.txt"}); err == nil {
		t.Fatal("snapshotWorkspace followed a symlink outside the workspace")
	}
}

func TestIsTextContentRejectsBinaryAndTerminalControlBytes(t *testing.T) {
	for _, value := range [][]byte{{0x00, 0xff}, {0x1b, '[', '2', 'J'}, {0xc2, 0x85}} {
		if isTextContent(value) {
			t.Errorf("isTextContent(%q) = true, want non-text content excluded", value)
		}
	}
	if !isTextContent([]byte("plain text\nwith\ttabs")) {
		t.Fatal("isTextContent rejected ordinary UTF-8 text")
	}
}

func TestBinarySnapshotStopsAfterBoundedClassificationProbe(t *testing.T) {
	reader := &probeCountingReader{remaining: 16 << 20}
	digest, omitted, err := hashSnapshotContent("large.bin", reader, 16<<20)
	if err != nil {
		t.Fatalf("hashSnapshotContent() error = %v", err)
	}
	if !omitted || digest != "" {
		t.Fatalf("hashSnapshotContent() = (%q, %t), want omitted without a digest", digest, omitted)
	}
	if reader.bytesRead != maxSnapshotScanBytes {
		t.Fatalf("binary probe read %d bytes, want bounded read of %d bytes", reader.bytesRead, maxSnapshotScanBytes)
	}
}

type probeCountingReader struct {
	remaining int64
	bytesRead int64
}

func (reader *probeCountingReader) Read(content []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	n := len(content)
	if int64(n) > reader.remaining {
		n = int(reader.remaining)
	}
	for index := 0; index < n; index++ {
		content[index] = 'a'
	}
	if reader.bytesRead == 0 && n > 0 {
		content[0] = 0
	}
	reader.bytesRead += int64(n)
	reader.remaining -= int64(n)
	return n, nil
}

func writeTestFile(t *testing.T, root, path string, content []byte) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatalf("create parent for %q: %v", path, err)
	}
	if err := os.WriteFile(absolute, content, 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
