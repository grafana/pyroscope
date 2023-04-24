//go:build embedassets
// +build embedassets

package public

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
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

func NewIndexHandler() (http.HandlerFunc, error) {
	indexPath := filepath.Join("build", "index.html")
	p, err := assets.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	return func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}, nil
}
