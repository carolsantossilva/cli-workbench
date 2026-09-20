package scaffold

import (
	"embed"
	"io/fs"
)

//go:embed all:templates
var embeddedTemplates embed.FS

func templates() (fs.FS, error) {
	return fs.Sub(embeddedTemplates, "templates")
}
