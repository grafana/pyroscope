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

	WriteResponseJSON(logger logrus.FieldLogger, w http.ResponseWriter, res interface{})
	WriteResponseFile(logger logrus.FieldLogger, w http.ResponseWriter, filename string, content []byte)
	WriteInvalidMethodError(logger logrus.FieldLogger, w http.ResponseWriter)
	WriteInvalidParameterError(logger logrus.FieldLogger, w http.ResponseWriter, err error)
	WriteInternalServerError(logger logrus.FieldLogger, w http.ResponseWriter, err error, msg string)
	WriteJSONEncodeError(logger logrus.FieldLogger, w http.ResponseWriter, err error)
	WriteError(logger logrus.FieldLogger, w http.ResponseWriter, code int, err error, msg string)
}
