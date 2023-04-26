package assets

import (
	"embed"
)

var (
	//go:embed index.html.tmpl
	Templates embed.FS

	//go:embed icon.png
	Icon []byte

	//go:embed materialize.min.css
	Stylesheet []byte
)
