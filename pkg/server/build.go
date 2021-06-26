package server

import (
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/build"
)

func (ctrl *Controller) buildHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(build.PrettyJSON()))
}
