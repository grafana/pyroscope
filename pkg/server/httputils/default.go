package httputils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/hashicorp/go-multierror"
	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/sirupsen/logrus"
)

type defaultErrorHandler struct {
}

func NewDefaultErrorHandler() *defaultErrorHandler {
	return &defaultErrorHandler{}
}

func (d *defaultErrorHandler) MustJSON(w http.ResponseWriter, v interface{}) {
	resp, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

// HandleError replies to the request with an appropriate message as
// JSON-encoded body and writes a corresponding message to the log
// with debug log level.
//
// Any error of a type not defined in this package or pkg/model, will be
// treated as an internal server error causing response code 500. Such
// errors are not sent but only logged with error log level.
func (d *defaultErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, logger logrus.FieldLogger, err error) {
	d.error(w, d.Logger(r, logger), err)
}

func (d *defaultErrorHandler) error(w http.ResponseWriter, logger logrus.FieldLogger, err error) {
	d.errorCode(w, logger, err, -1)
}

// errorCode replies to the request with the specified error message
// as JSON-encoded body and writes corresponding message to the log.
//
// If HTTP code is less than or equal zero, it will be deduced based on
// the error. If it fails, StatusInternalServerError will be returned
// without the response body. The error can be of 'multierror.Error' type.
//
// The call writes messages with the debug log level except the case
// when the code is StatusInternalServerError which is logged as an error.
//
// It does not end the HTTP request; the caller should ensure no further
// writes are done to w.
func (d *defaultErrorHandler) errorCode(w http.ResponseWriter, logger logrus.FieldLogger, err error, code int) {
	switch {
	case err == nil:
		return
	case code > 0:
	case model.IsAuthenticationError(err):
		code = http.StatusUnauthorized
		err = model.ErrCredentialsInvalid
	case model.IsAuthorizationError(err):
		code = http.StatusForbidden
	case model.IsValidationError(err):
		code = http.StatusBadRequest
	case model.IsNotFoundError(err):
		code = http.StatusNotFound
	case IsJSONError(err):
		code = http.StatusBadRequest
		switch {
		case errors.Is(err, io.EOF):
			err = ErrRequestBodyRequired
		case errors.Is(err, io.ErrUnexpectedEOF):
			// https://github.com/golang/go/issues/25956
			err = ErrRequestBodyJSONInvalid
		}
	default:
		// No response code provided and it can't be determined.
		code = http.StatusInternalServerError
	}

	var e Errors
	if m := new(multierror.Error); errors.As(err, &m) {
		m.ErrorFormat = listFormatFunc
		for _, x := range m.Errors {
			e.Errors = append(e.Errors, x.Error())
		}
	} else {
		e.Errors = []string{err.Error()}
	}

	w.WriteHeader(code)
	if logger != nil {
		// Internal errors must not be shown to users but
		// logged with error log level.
		logger = logger.WithError(err).WithField("code", code)
		msg := strings.ToLower(http.StatusText(code))
		if code == http.StatusInternalServerError {
			logger.Error(msg)
			return
		}
		logger.Debug(msg)
	}

	d.MustJSON(w, e)
}

var (
	errParamIDRequired = model.ValidationError{Err: errors.New("id parameter is required")}
)

var (
	ErrRequestBodyRequired    = model.ValidationError{Err: errors.New("request body required")}
	ErrRequestBodyJSONInvalid = model.ValidationError{Err: errors.New("request body contains malformed JSON")}
)

type Errors struct {
	Errors []string `json:"errors"`
}

func listFormatFunc(es []error) string {
	if len(es) == 1 {
		return es[0].Error()
	}
	points := make([]string, len(es))
	for i, err := range es {
		points[i] = err.Error()
	}
	return strings.Join(points, "; ")
}

func (d *defaultErrorHandler) IdFromRequest(r *http.Request) (uint, error) {
	v, ok := mux.Vars(r)["id"]
	if !ok {
		return 0, errParamIDRequired
	}
	id, err := strconv.ParseUint(v, 10, 0)
	if err != nil {
		return 0, model.ValidationError{Err: fmt.Errorf("id parameter is invalid: %w", err)}
	}
	return uint(id), nil
}

// Logger creates a new logger scoped to the request
// and enriches it with the known fields.
func (d *defaultErrorHandler) Logger(r *http.Request, logger logrus.FieldLogger) logrus.FieldLogger {
	fields := logrus.Fields{
		"url":    r.URL.String(),
		"method": r.Method,
		"remote": r.RemoteAddr,
	}
	u, ok := model.UserFromContext(r.Context())
	if ok {
		fields["user"] = u.Name
	}
	var k model.APIKey
	k, ok = model.APIKeyFromContext(r.Context())
	if ok {
		fields["api_key"] = k.Name
	}
	return logger.WithFields(fields)
}

func (d *defaultErrorHandler) WriteResponseJSON(log logrus.FieldLogger, w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		d.WriteJSONEncodeError(log, w, err)
	}
}

func (d *defaultErrorHandler) WriteResponseFile(_ logrus.FieldLogger, w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
	w.(http.Flusher).Flush()
}

func (d *defaultErrorHandler) WriteInvalidMethodError(log logrus.FieldLogger, w http.ResponseWriter) {
	d.writeErrorMessage(log, w, http.StatusMethodNotAllowed, "method not allowed")
}

func (d *defaultErrorHandler) WriteInvalidParameterError(log logrus.FieldLogger, w http.ResponseWriter, err error) {
	d.WriteError(log, w, http.StatusBadRequest, err, "invalid parameter")
}

func (d *defaultErrorHandler) WriteInternalServerError(log logrus.FieldLogger, w http.ResponseWriter, err error, msg string) {
	d.WriteError(log, w, http.StatusInternalServerError, err, msg)
}

func (d *defaultErrorHandler) WriteJSONEncodeError(log logrus.FieldLogger, w http.ResponseWriter, err error) {
	d.WriteInternalServerError(log, w, err, "encoding response body")
}

func (d *defaultErrorHandler) WriteError(log logrus.FieldLogger, w http.ResponseWriter, code int, err error, msg string) {
	log.WithError(err).Error(msg)
	d.writeMessage(w, code, "%s: %q", msg, err)
}

func (d *defaultErrorHandler) writeErrorMessage(log logrus.FieldLogger, w http.ResponseWriter, code int, msg string) {
	log.Error(msg)
	d.writeMessage(w, code, msg)
}

func (d *defaultErrorHandler) writeMessage(w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
