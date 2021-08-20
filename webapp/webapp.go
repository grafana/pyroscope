package webapp

import (
	"embed"
	"io/fs"
	"net/http"
)

// Asterisk here is important, see https://github.com/golang/go/issues/42328 for more details
//go:embed public/*
var assets embed.FS

func Assets() (http.FileSystem, error) {
	fsys, err := fs.Sub(assets, "public")

	if err != nil {
		return nil, err
	}

	return http.FS(fsys), nil
}
