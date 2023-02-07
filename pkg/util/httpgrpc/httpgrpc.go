package httpgrpc

import (
	"fmt"

	"github.com/golang/protobuf/ptypes"
	log "github.com/sirupsen/logrus"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// Errorf returns a HTTP gRPC error than is correctly forwarded over
// gRPC, and can eventually be converted back to a HTTP response with
// HTTPResponseFromError.
func Errorf(code int, tmpl string, args ...interface{}) error {
	return ErrorFromHTTPResponse(&HTTPResponse{
		Code: int32(code),
		Body: []byte(fmt.Sprintf(tmpl, args...)),
	})
}

// ErrorFromHTTPResponse converts an HTTP response into a grpc error
func ErrorFromHTTPResponse(resp *HTTPResponse) error {
	s := status.New(codes.Code(resp.Code), string(resp.Body))
	s, err := s.WithDetails(resp)
	if err != nil {
		return err
	}
	return status.ErrorProto(s.Proto())
}

// HTTPResponseFromError converts a grpc error into an HTTP response
func HTTPResponseFromError(err error) (*HTTPResponse, bool) {
	s, ok := status.FromError(err)
	if !ok {
		return nil, false
	}

	status := s.Proto()
	if len(status.Details) != 1 {
		return nil, false
	}

	var resp HTTPResponse
	if err := ptypes.UnmarshalAny(status.Details[0], &resp); err != nil {
		log.Errorf("Got error containing non-response: %v", err)
		return nil, false
	}

	return &resp, true
}
