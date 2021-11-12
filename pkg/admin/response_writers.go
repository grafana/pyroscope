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

func (ctrl *Controller) writeError(w http.ResponseWriter, code int, err error, msg string) {
	ctrl.log.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func (ctrl *Controller) writeResponseJSON(w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		ctrl.writeError(w, http.StatusInternalServerError, err, "encoding response body")
	}
}
