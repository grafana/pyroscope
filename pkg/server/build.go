package server

import (
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/build"
)

func (*Controller) buildHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(build.PrettyJSON()))
}
