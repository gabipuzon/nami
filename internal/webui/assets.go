//go:build webui

package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:assets
var assets embed.FS

func Files() fs.FS {
	files, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	return files
}
