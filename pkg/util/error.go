package util

import (
	"context"
	"errors"
	"net/http"

	"github.com/weaveworks/common/httpgrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/fire/pkg/tenant"
)

// StatusClientClosedRequest is the status code for when a client request cancellation of an http request
const StatusClientClosedRequest = 499

const (
	ErrClientCanceled   = "The request was cancelled by the client."
	ErrDeadlineExceeded = "Request timed out, decrease the duration of the request or add more label matchers (prefer exact match over regex match) to reduce the amount of data processed."
)

// WriteError write a go error with the correct status code.
func WriteError(err error, w http.ResponseWriter) {
	status, cerr := ClientHTTPStatusAndError(err)
	http.Error(w, cerr.Error(), status)
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
