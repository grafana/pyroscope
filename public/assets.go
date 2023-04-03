//go:build !embedassets
// +build !embedassets

package public

import (
	"net/http"
)

var AssetsEmbedded = false

func Assets() (http.FileSystem, error) {
	return http.Dir("./public/build"), nil
}
