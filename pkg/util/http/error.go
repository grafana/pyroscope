package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/httpgrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

var errorWriter = connect.NewErrorWriter()

// StatusClientClosedRequest is the status code for when a client request cancellation of an http request
const StatusClientClosedRequest = 499

const (
	ErrClientCanceled   = "The request was cancelled by the client."
	ErrDeadlineExceeded = "Request timed out, decrease the duration of the request or add more label matchers (prefer exact match over regex match) to reduce the amount of data processed."
)

// Error write a go error with the correct status code.
func Error(w http.ResponseWriter, err error) {
	var connectErr *connect.Error
	if ok := errors.As(err, &connectErr); ok {
		writeErr := errorWriter.Write(w, &http.Request{
			Header: http.Header{"Content-Type": []string{"application/json"}},
		}, err)
		if writeErr != nil {
			http.Error(w, writeErr.Error(), http.StatusInternalServerError)
		}
		return
	}
	status, cerr := ClientHTTPStatusAndError(err)
	ErrorWithStatus(w, cerr, status)
}

func ErrorWithStatus(w http.ResponseWriter, err error, status int) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(struct {
		Code    connect.Code `json:"code"`
		Message string       `json:"message"`
	}{
		Code:    connectgrpc.HTTPToCode(int32(status)),
		Message: err.Error(),
	}); err != nil {
		http.Error(w, err.Error(), status)
	}
}

// ClientHTTPStatusAndError returns error and http status that is "safe" to return to client without
// exposing any implementation details.
func ClientHTTPStatusAndError(err error) (int, error) {
	// todo handle multi errors
	// me, ok := err.(multierror.MultiError)
	// if ok && me.Is(context.Canceled) {
	// 	return StatusClientClosedRequest, errors.New(ErrClientCanceled)
	// }
	// if ok && me.IsDeadlineExceeded() {
	// 	return http.StatusGatewayTimeout, errors.New(ErrDeadlineExceeded)
	// }

	s, isRPC := status.FromError(err)
	switch {
	case errors.Is(err, context.Canceled):
		return StatusClientClosedRequest, errors.New(ErrClientCanceled)
	case errors.Is(err, context.DeadlineExceeded) ||
		(isRPC && s.Code() == codes.DeadlineExceeded):
		return http.StatusGatewayTimeout, errors.New(ErrDeadlineExceeded)
	case errors.Is(err, tenant.ErrNoTenantID):
		return http.StatusBadRequest, err
	default:
		if grpcErr, ok := httpgrpc.HTTPResponseFromError(err); ok {
			return int(grpcErr.Code), errors.New(string(grpcErr.Body))
		}
		return http.StatusInternalServerError, err
	}
}
