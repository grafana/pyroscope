package server

import (
	"fmt"
	"io"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
)

type Payload struct {
	Name    string
	Profile string
	Type    string
}

func (ctrl *Controller) exportHandler() http.HandlerFunc {
	return NewExportHandler(ctrl.httpUtils)
}

func NewExportHandler(httpUtils httputils.Utils) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Post("https://flamegraph.com/api/upload/v1", "application/json", r.Body)
		if err != nil {
			httpUtils.WriteError(r, w, 500, err, fmt.Sprintf("could not upload profile: %v", err))
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.Copy(w, resp.Body)
	}
}
