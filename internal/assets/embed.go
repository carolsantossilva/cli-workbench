package assets

import _ "embed"

//go:embed workspace.schema.json
var workspaceSchemaFile []byte

func WorkspaceSchema() []byte {
	return append([]byte(nil), workspaceSchemaFile...)
}
