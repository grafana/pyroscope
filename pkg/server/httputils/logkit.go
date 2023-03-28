package httputils

import (
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"net/http"
)

type logKitImpl struct {
	l log.Logger
}

func NewLogKitErrorUtils(l log.Logger) ErrorUtils {
	return &logKitImpl{
		l: level.Error(l),
	}
}

func (i *logKitImpl) WriteInvalidMethodError(r *http.Request, w http.ResponseWriter) {
	i.writeErrorMessage(r, w, http.StatusMethodNotAllowed, "method not allowed")
}

func (i *logKitImpl) WriteInvalidParameterError(r *http.Request, w http.ResponseWriter, err error) {
	i.WriteError(r, w, http.StatusBadRequest, err, "invalid parameter")
}

func (i *logKitImpl) WriteInternalServerError(r *http.Request, w http.ResponseWriter, err error, msg string) {
	i.WriteError(r, w, http.StatusInternalServerError, err, msg)
}

func (i *logKitImpl) WriteJSONEncodeError(r *http.Request, w http.ResponseWriter, err error) {
	i.WriteInternalServerError(r, w, err, "encoding response body")
}

func (i *logKitImpl) WriteError(r *http.Request, w http.ResponseWriter, code int, err error, msg string) {
	_ = i.l.Log(
		"err", err,
		"msg", msg,
	)
	i.writeMessage(r, w, code, "%s: %q", msg, err)
}

func (i *logKitImpl) writeErrorMessage(r *http.Request, w http.ResponseWriter, code int, msg string) {
	_ = i.l.Log(msg)
	i.writeMessage(r, w, code, msg)
}

func (*logKitImpl) writeMessage(_ *http.Request, w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
