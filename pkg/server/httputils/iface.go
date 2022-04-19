package httputils

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

// TODO(petethepig): this interface is pretty large, we can probably simplify it, some methods do pretty similar things
type Utils interface {
	// these methods were originally extracted from api package
	MustJSON(r *http.Request, w http.ResponseWriter, v interface{})
	HandleError(r *http.Request, w http.ResponseWriter, err error)
	IDFromRequest(r *http.Request) (uint, error)
	Logger(r *http.Request) logrus.FieldLogger

	// these methods were originally extracted from server package
	WriteResponseJSON(r *http.Request, w http.ResponseWriter, res interface{})
	WriteResponseFile(r *http.Request, w http.ResponseWriter, filename string, content []byte)
	WriteInvalidMethodError(r *http.Request, w http.ResponseWriter)
	WriteInvalidParameterError(r *http.Request, w http.ResponseWriter, err error)
	WriteInternalServerError(r *http.Request, w http.ResponseWriter, err error, msg string)
	WriteJSONEncodeError(r *http.Request, w http.ResponseWriter, err error)
	WriteError(r *http.Request, w http.ResponseWriter, code int, err error, msg string)
}
