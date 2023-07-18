//go:build embedassets
// +build embedassets

package public

import (
	"embed"
	"fmt"
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

// NewIndexHandler parses and executes the webpack-built index.html
// Then returns a handler that serves that templated file
func NewIndexHandler(basePath string) (http.HandlerFunc, error) {
	indexPath := filepath.Join("build", "index.html")
	p, err := assets.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	buf, err := ExecuteTemplate(p, Params{
		BasePath: basePath,
	})
	if err != nil {
		return nil, fmt.Errorf("could not execute template: %v", err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		_, err := w.Write(buf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}, nil
}
