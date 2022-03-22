package api

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
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

var (
	ErrRequestBodyRequired    = model.ValidationError{Err: errors.New("request body required")}
	ErrRequestBodyJSONInvalid = model.ValidationError{Err: errors.New("request body contains malformed JSON")}
)

type Errors struct {
	Errors []string `json:"errors"`
}

type JSONError struct {
	Err error
}

func (e JSONError) Error() string { return e.Err.Error() }

func (e JSONError) Unwrap() error { return e.Err }

func IsJSONError(err error) bool {
	if err == nil {
		return false
	}
	var v JSONError
	return errors.As(err, &v)
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

// HandleError replies to the request with an appropriate message as
// JSON-encoded body and writes a corresponding message to the log
// with debug log level.
//
// Any error of a type not defined in this package or pkg/model, will be
// treated as an internal server error causing response code 500. Such
// errors are not sent but only logged with error log level.
func HandleError(w http.ResponseWriter, r *http.Request, logger logrus.FieldLogger, err error) {
	Error(w, Logger(r, logger), err)
}

// Logger creates a new logger scoped to the request
// and enriches it with the known fields.
func Logger(r *http.Request, logger logrus.FieldLogger) logrus.FieldLogger {
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

func Error(w http.ResponseWriter, logger logrus.FieldLogger, err error) {
	ErrorCode(w, logger, err, -1)
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
func ErrorCode(w http.ResponseWriter, logger logrus.FieldLogger, err error, code int) {
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

	MustJSON(w, e)
}

func MustJSON(w http.ResponseWriter, v interface{}) {
	resp, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

var (
	errParamIDRequired = model.ValidationError{Err: errors.New("id parameter is required")}
)

func idFromRequest(r *http.Request) (uint, error) {
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
