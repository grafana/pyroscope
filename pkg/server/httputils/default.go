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

type DefaultImpl struct {
	logger logrus.FieldLogger
}

func NewDefaultHelper(logger logrus.FieldLogger) *DefaultImpl {
	return &DefaultImpl{
		logger: logger,
	}
}

func (*DefaultImpl) MustJSON(_ *http.Request, w http.ResponseWriter, v interface{}) {
	resp, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

func (*DefaultImpl) mustJSONError(_ *http.Request, w http.ResponseWriter, code int, v interface{}) {
	resp, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(resp)
}

// HandleError replies to the request with an appropriate message as
// JSON-encoded body and writes a corresponding message to the log
// with debug log level.
//
// Any error of a type not defined in this package or pkg/model, will be
// treated as an internal server error causing response code 500. Such
// errors are not sent but only logged with error log level.
func (d *DefaultImpl) HandleError(r *http.Request, w http.ResponseWriter, err error) {
	d.ErrorCode(r, w, d.Logger(r), err, -1)
}

// ErrorCode replies to the request with the specified error message
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
func (d *DefaultImpl) ErrorCode(r *http.Request, w http.ResponseWriter, logger logrus.FieldLogger, err error, code int) {
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

	if logger != nil {
		// Internal errors must not be shown to users but
		// logged with error log level.
		logger = logger.WithError(err).WithField("code", code)
		msg := strings.ToLower(http.StatusText(code))
		if code == http.StatusInternalServerError {
			w.WriteHeader(code)
			logger.Error(msg)
			return
		}
		logger.Debug(msg)
	}

	d.mustJSONError(r, w, code, e)
}

var (
	ErrParamIDRequired        = model.ValidationError{Err: errors.New("id parameter is required")}
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

func (*DefaultImpl) IDFromRequest(r *http.Request) (uint, error) {
	v, ok := mux.Vars(r)["id"]
	if !ok {
		return 0, ErrParamIDRequired
	}
	id, err := strconv.ParseUint(v, 10, 0)
	if err != nil {
		return 0, model.ValidationError{Err: fmt.Errorf("id parameter is invalid: %w", err)}
	}
	return uint(id), nil
}

// Logger creates a new logger scoped to the request
// and enriches it with the known fields.
func (d *DefaultImpl) Logger(r *http.Request) logrus.FieldLogger {
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
	return d.logger.WithFields(fields)
}

func (d *DefaultImpl) WriteResponseJSON(r *http.Request, w http.ResponseWriter, res interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		d.WriteJSONEncodeError(r, w, err)
	}
}

func (*DefaultImpl) WriteResponseFile(_ *http.Request, w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%v", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
	w.(http.Flusher).Flush()
}

func (d *DefaultImpl) WriteInvalidMethodError(r *http.Request, w http.ResponseWriter) {
	d.writeErrorMessage(r, w, http.StatusMethodNotAllowed, "method not allowed")
}

func (d *DefaultImpl) WriteInvalidParameterError(r *http.Request, w http.ResponseWriter, err error) {
	d.WriteError(r, w, http.StatusBadRequest, err, "invalid parameter")
}

func (d *DefaultImpl) WriteInternalServerError(r *http.Request, w http.ResponseWriter, err error, msg string) {
	d.WriteError(r, w, http.StatusInternalServerError, err, msg)
}

func (d *DefaultImpl) WriteJSONEncodeError(r *http.Request, w http.ResponseWriter, err error) {
	d.WriteInternalServerError(r, w, err, "encoding response body")
}

func (d *DefaultImpl) WriteError(r *http.Request, w http.ResponseWriter, code int, err error, msg string) {
	d.logger.WithError(err).Error(msg)
	d.writeMessage(r, w, code, "%s: %q", msg, err)
}

func (d *DefaultImpl) writeErrorMessage(r *http.Request, w http.ResponseWriter, code int, msg string) {
	d.logger.Error(msg)
	d.writeMessage(r, w, code, msg)
}

func (*DefaultImpl) writeMessage(_ *http.Request, w http.ResponseWriter, code int, format string, args ...interface{}) {
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprintln(w)
}
