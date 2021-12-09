package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/hashicorp/go-multierror"
	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

var (
	errParamIDRequired     = model.ValidationError{Err: errors.New("id parameter is required")}
	errParamIDInvalid      = model.ValidationError{Err: errors.New("id parameter is invalid")}
	errRequestBodyRequired = model.ValidationError{Err: errors.New("request body required")}
	errRequestBodyInvalid  = model.ValidationError{Err: errors.New("request body contains malformed JSON")}
)

type Errors struct {
	Errors []string `json:"errors"`
}

func DecodeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, io.EOF):
		err = errRequestBodyRequired
	case errors.Is(err, io.ErrUnexpectedEOF):
		// https://github.com/golang/go/issues/25956
		err = errRequestBodyInvalid
	}
	Error(w, model.ValidationError{Err: err})
}

func Error(w http.ResponseWriter, err error) {
	ErrorCode(w, err, -1)
}

// ErrorCode replies to the request with the specified error message
// as JSON-encoded body.
//
// If HTTP code is less than 0, it will be deduced based on the error.
// If it fails, StatusInternalServerError will be returned without the
// response body. The error can be of multierror.Error type.
//
// It does not otherwise end the request; the caller should ensure
// no further writes are done to w.
func ErrorCode(w http.ResponseWriter, err error, code int) {
	switch {
	case err == nil:
		return
	case code > 0:
		w.WriteHeader(code)
	case model.IsValidationError(err):
		w.WriteHeader(http.StatusBadRequest)
	case model.IsNotFoundError(err):
		w.WriteHeader(http.StatusNotFound)
	default:
		// No response code provided and it can be determined.
		// Internal errors must not be shown to users.
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var e Errors
	if m := new(multierror.Error); errors.As(err, &m) {
		for _, x := range m.Errors {
			e.Errors = append(e.Errors, x.Error())
		}
	} else {
		e.Errors = []string{err.Error()}
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
