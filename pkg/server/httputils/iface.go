package httputils

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

// TODO(petethepig): this interface is pretty large, we can probably simplify it
type Utils interface {
	// these methods were originally extracted from api package
	MustJSON(w http.ResponseWriter, v interface{})
	HandleError(w http.ResponseWriter, r *http.Request, err error)
	IdFromRequest(r *http.Request) (uint, error)
	Logger(r *http.Request) logrus.FieldLogger

	// these methods were originally extracted from server package
	WriteResponseJSON(w http.ResponseWriter, res interface{})
	WriteResponseFile(w http.ResponseWriter, filename string, content []byte)
	WriteInvalidMethodError(w http.ResponseWriter)
	WriteInvalidParameterError(w http.ResponseWriter, err error)
	WriteInternalServerError(w http.ResponseWriter, err error, msg string)
	WriteJSONEncodeError(w http.ResponseWriter, err error)
	WriteError(w http.ResponseWriter, code int, err error, msg string)
}
