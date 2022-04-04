package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
)

func WriteResponseJSON(log *logrus.Logger, w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		WriteJSONEncodeError(log, w, err)
	}
}

func WriteResponseFile(_ *logrus.Logger, w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
	w.(http.Flusher).Flush()
}

func WriteInvalidMethodError(log *logrus.Logger, w http.ResponseWriter) {
	WriteErrorMessage(log, w, http.StatusMethodNotAllowed, "method not allowed")
}

func WriteInvalidParameterError(log *logrus.Logger, w http.ResponseWriter, err error) {
	WriteError(log, w, http.StatusBadRequest, err, "invalid parameter")
}

func WriteInternalServerError(log *logrus.Logger, w http.ResponseWriter, err error, msg string) {
	WriteError(log, w, http.StatusInternalServerError, err, msg)
}

func WriteJSONEncodeError(log *logrus.Logger, w http.ResponseWriter, err error) {
	WriteInternalServerError(log, w, err, "encoding response body")
}

func WriteError(log *logrus.Logger, w http.ResponseWriter, code int, err error, msg string) {
	log.WithError(err).Error(msg)
	writeMessage(w, code, "%s: %q", msg, err)
}

func WriteErrorMessage(log *logrus.Logger, w http.ResponseWriter, code int, msg string) {
	log.Error(msg)
	writeMessage(w, code, msg)
}

func writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
