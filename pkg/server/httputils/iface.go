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
}
