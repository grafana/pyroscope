//go:build embedassets
// +build embedassets

package public

import (
	"embed"
	"io/fs"
	"net/http"
)

var AssetsEmbedded = true

//go:embed build
var assets embed.FS

func Assets() (http.FileSystem, error) {
	fsys, err := fs.Sub(assets, "build")

	if err != nil {
		return nil, err
	}

	return http.FS(fsys), nil
}
