package server

import (
	"fmt"
	"io"
	"net/http"

	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/sirupsen/logrus"
)

type Payload struct {
	Name    string
	Profile string
	Type    string
}

func (ctrl *Controller) exportHandler() http.HandlerFunc {
	return NewExportHandler(ctrl.log, ctrl.httpUtils)
}

func NewExportHandler(log *logrus.Logger, httpUtils httputils.Helper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Post("https://flamegraph.com/api/upload/v1", "application/json", r.Body)
		if err != nil {
			httpUtils.WriteError(log, w, 500, err, fmt.Sprintf("could not upload profile: %v", err))
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.Copy(w, resp.Body)
	}
}
