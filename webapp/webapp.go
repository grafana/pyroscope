package webapp

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed public
var assets embed.FS

func Assets() (http.FileSystem, error) {
	fsys, err := fs.Sub(assets, "public")

	if err != nil {
		return nil, err
	}

	return http.FS(fsys), nil
}
