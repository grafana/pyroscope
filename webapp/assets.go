// +build !embedassets

package webapp

import (
	"net/http"
)

var AssetsEmbedded = false

func Assets() (http.FileSystem, error) {
	return http.Dir("./webapp/public"), nil
}
