package webapp

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed public
var EmbeddedAssets embed.FS

func GetFileSystem () http.FileSystem {
fsys, err := fs.Sub(EmbeddedAssets, "public")

	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}
