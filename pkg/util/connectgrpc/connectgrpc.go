package connectgrpc

import (
	"context"
	"errors"
	"net/http"

	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/util/httpgrpc"
)

type UnaryHandler[Req any, Res any] func(context.Context, *connect.Request[Req]) (*connect.Response[Res], error)

func HandleUnary[Req any, Res any](ctx context.Context, req *httpgrpc.HTTPRequest, u UnaryHandler[Req, Res]) (*httpgrpc.HTTPResponse, error) {
	connectReq, err := decodeRequest[Req](req)
	if err != nil {
		return nil, err
	}
	connectResp, err := u(ctx, connectReq)
	if err != nil {
		if errors.Is(err, tenant.ErrNoTenantID) {
			err = connect.NewError(connect.CodeUnauthenticated, err)
		}
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			return &httpgrpc.HTTPResponse{
				Code:    CodeToHTTP(connectErr.Code()),
				Body:    []byte(connectErr.Message()),
				Headers: connectHeaderToHTTPHeader(connectErr.Meta()),
			}, nil
		}

		return nil, err
	}
	return encodeResponse(connectResp)
}

type GRPCRoundTripper interface {
	RoundTripGRPC(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error)
}

type GRPCHandler interface {
	Handle(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error)
}

func RoundTripUnary[Req any, Res any](rt GRPCRoundTripper, ctx context.Context, in *connect.Request[Req]) (*connect.Response[Res], error) {
	req, err := encodeRequest(in)
	if err != nil {
		return nil, err
	}
	res, err := rt.RoundTripGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Code/100 != 2 {
		return nil, connect.NewError(HTTPToCode(res.Code), errors.New(string(res.Body)))
	}
	result := &connect.Response[Res]{
		Msg: new(Res),
	}

	err = proto.Unmarshal(res.Body, result.Any().(proto.Message))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func encodeResponse[Req any](resp *connect.Response[Req]) (*httpgrpc.HTTPResponse, error) {
	out := &httpgrpc.HTTPResponse{
		Headers: connectHeaderToHTTPHeader(resp.Header()),
		Code:    http.StatusOK,
	}
	var err error
	out.Body, err = proto.Marshal(resp.Any().(proto.Message))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func connectHeaderToHTTPHeader(header http.Header) []*httpgrpc.Header {
	result := []*httpgrpc.Header{}
	for k, v := range header {
		result = append(result, &httpgrpc.Header{
			Key:    k,
			Values: v,
		})
	}
	return result
}

func decodeRequest[Req any](req *httpgrpc.HTTPRequest) (*connect.Request[Req], error) {
	result := &connect.Request[Req]{
		Msg: new(Req),
	}
	err := proto.Unmarshal(req.Body, result.Any().(proto.Message))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func encodeRequest[Req any](req *connect.Request[Req]) (*httpgrpc.HTTPRequest, error) {
	out := &httpgrpc.HTTPRequest{
		Method:  http.MethodPost,
		Url:     req.Spec().Procedure,
		Headers: []*httpgrpc.Header{},
	}
	for k, v := range req.Header() {
		out.Headers = append(out.Headers, &httpgrpc.Header{Key: k, Values: v})
	}
	var err error
	msg := req.Any()
	out.Body, err = proto.Marshal(msg.(proto.Message))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func CodeToHTTP(code connect.Code) int32 {
	// Return literals rather than named constants from the HTTP package to make
	// it easier to compare this function to the Connect specification.
	switch code {
	case connect.CodeCanceled:
		return 499
	case connect.CodeUnknown:
		return 500
	case connect.CodeInvalidArgument:
		return 400
	case connect.CodeDeadlineExceeded:
		return 504
	case connect.CodeNotFound:
		return 404
	case connect.CodeAlreadyExists:
		return 409
	case connect.CodePermissionDenied:
		return 403
	case connect.CodeResourceExhausted:
		return 429
	case connect.CodeFailedPrecondition:
		return 412
	case connect.CodeAborted:
		return 409
	case connect.CodeOutOfRange:
		return 400
	case connect.CodeUnimplemented:
		return 404
	case connect.CodeInternal:
		return 500
	case connect.CodeUnavailable:
		return 503
	case connect.CodeDataLoss:
		return 500
	case connect.CodeUnauthenticated:
		return 401
	default:
		return 500 // same as CodeUnknown
	}
}

func HTTPToCode(httpCode int32) connect.Code {
	// As above, literals are easier to compare to the specificaton (vs named
	// constants).
	switch httpCode {
	case 400:
		return connect.CodeInvalidArgument
	case 401:
		return connect.CodeUnauthenticated
	case 403:
		return connect.CodePermissionDenied
	case 404:
		return connect.CodeUnimplemented
	case 412:
		return connect.CodeFailedPrecondition
	case 413:
		return connect.CodeInvalidArgument
	case 429:
		return connect.CodeResourceExhausted
	case 431:
		return connect.CodeResourceExhausted
	case 499:
		return connect.CodeCanceled
	case 502, 503:
		return connect.CodeUnavailable
	case 504:
		return connect.CodeDeadlineExceeded
	default:
		return connect.CodeUnknown
	}
}
