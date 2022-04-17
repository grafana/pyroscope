package httputils

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

type Helper interface {
	MustJSON(w http.ResponseWriter, v interface{})
	HandleError(w http.ResponseWriter, r *http.Request, logger logrus.FieldLogger, err error)
	Error(w http.ResponseWriter, logger logrus.FieldLogger, err error)
	IdFromRequest(r *http.Request) (uint, error)
	Logger(r *http.Request, logger logrus.FieldLogger) logrus.FieldLogger

	WriteResponseJSON(log *logrus.Logger, w http.ResponseWriter, res interface{})
	WriteResponseFile(_ *logrus.Logger, w http.ResponseWriter, filename string, content []byte)
	WriteInvalidMethodError(log *logrus.Logger, w http.ResponseWriter)
	WriteInvalidParameterError(log *logrus.Logger, w http.ResponseWriter, err error)
	WriteInternalServerError(log *logrus.Logger, w http.ResponseWriter, err error, msg string)
	WriteJSONEncodeError(log *logrus.Logger, w http.ResponseWriter, err error)
	WriteError(log *logrus.Logger, w http.ResponseWriter, code int, err error, msg string)
	WriteErrorMessage(log *logrus.Logger, w http.ResponseWriter, code int, msg string)
}
