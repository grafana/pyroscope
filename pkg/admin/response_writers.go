package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Helper functions for writing the response
func writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)

	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}

func (ctrl *Controller) writeJSONEncodeError(w http.ResponseWriter, err error) {
	ctrl.writeInternalServerError(w, err, "encoding response body")
}

func (ctrl *Controller) writeInternalServerError(w http.ResponseWriter, err error, msg string) {
	ctrl.writeError(w, http.StatusInternalServerError, err, msg)
}

func (ctrl *Controller) writeErrorMessage(w http.ResponseWriter, code int, msg string) {
	ctrl.log.Error(msg)
	writeMessage(w, code, msg)
}

func (ctrl *Controller) writeError(w http.ResponseWriter, code int, err error, msg string) {
	ctrl.log.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func (ctrl *Controller) writeResponseJSON(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		ctrl.writeJSONEncodeError(w, err)
	}
}
