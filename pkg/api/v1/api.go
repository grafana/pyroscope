package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/hashicorp/go-multierror"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

var (
	errParamIDRequired = model.ValidationError{Err: errors.New("id parameter is required")}
	errParamIDInvalid  = model.ValidationError{Err: errors.New("id parameter is invalid")}
)

type Error struct {
	Errors []error `json:"errors"`
}

func idFromRequest(r *http.Request) (uint, error) {
	v, ok := mux.Vars(r)["id"]
	if !ok {
		return 0, errParamIDRequired
	}
	id, err := strconv.ParseUint(v, 10, 0)
	if err != nil {
		return 0, errParamIDInvalid
	}
	return uint(id), nil
}

func respondWithError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case model.IsValidationError(err):
		w.WriteHeader(http.StatusBadRequest)
	case model.IsNotFoundError(err):
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		// Internal errors must not be shown.
		return
	}
	var e Error
	if m := new(multierror.Error); errors.As(err, &m) {
		e.Errors = m.Errors
	} else {
		e.Errors = []error{err}
	}
	respondWithJSON(w, e)
}

func respondWithJSON(w http.ResponseWriter, v interface{}) {
	resp, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}
