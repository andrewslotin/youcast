package assets

import (
	"embed"
)

var (
	//go:embed index.html.tmpl
	Templates embed.FS

	//go:embed icon.png
	Icon []byte
)
