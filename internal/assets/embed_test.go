package assets

import (
	"bytes"
	"testing"
)

func TestWorkspaceSchemaIsEmbeddedVerbatim(t *testing.T) {
	// This catches a build that omits or changes the runtime schema asset.
	if !bytes.Equal(WorkspaceSchema(), workspaceSchemaFile) {
		t.Fatal("embedded workspace schema differs from the canonical asset")
	}
}
