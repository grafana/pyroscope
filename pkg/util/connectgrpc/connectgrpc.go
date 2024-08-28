package connectgrpc

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util/httpgrpc"
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
				Headers: connectHeaderToHTTPGRPCHeader(connectErr.Meta()),
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

func RoundTripUnary[Req any, Res any](ctx context.Context, rt GRPCRoundTripper, in *connect.Request[Req]) (*connect.Response[Res], error) {
	req, err := encodeRequest(ctx, in)
	if err != nil {
		return nil, err
	}
	res, err := rt.RoundTripGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Code/100 != 2 {
		err := connect.NewError(HTTPToCode(res.Code), errors.New(string(res.Body)))
		for _, h := range res.Headers {
			for _, v := range h.Values {
				err.Meta().Add(h.Key, v)
			}
		}
		return nil, err
	}
	return decodeResponse[Res](res)
}

func CloneRequest[Req any](base *connect.Request[Req], msg *Req) *connect.Request[Req] {
	r := *base
	r.Msg = msg
	return &r
}

func encodeResponse[Req any](resp *connect.Response[Req]) (*httpgrpc.HTTPResponse, error) {
	out := &httpgrpc.HTTPResponse{
		Headers: connectHeaderToHTTPGRPCHeader(resp.Header()),
		Code:    http.StatusOK,
	}
	var err error
	out.Body, err = proto.Marshal(resp.Any().(proto.Message))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func connectHeaderToHTTPGRPCHeader(header http.Header) []*httpgrpc.Header {
	result := make([]*httpgrpc.Header, 0, len(header))
	for k, v := range header {
		result = append(result, &httpgrpc.Header{
			Key:    k,
			Values: v,
		})
	}
	return result
}

func httpgrpcHeaderToConnectHeader(header []*httpgrpc.Header) http.Header {
	result := make(http.Header, len(header))
	for _, h := range header {
		result[h.Key] = h.Values
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

type connectURLCtxKey struct{}

func WithProcedure(ctx context.Context, u string) context.Context {
	return context.WithValue(ctx, connectURLCtxKey{}, u)
}

func ProcedureFromContext(ctx context.Context) string {
	s, _ := ctx.Value(connectURLCtxKey{}).(string)
	return s
}

func encodeRequest[Req any](ctx context.Context, req *connect.Request[Req]) (*httpgrpc.HTTPRequest, error) {
	url := ProcedureFromContext(ctx)
	if url == "" {
		if url = req.Spec().Procedure; url == "" {
			return nil, errors.New("cannot encode a request with empty procedure")
		}
	}
	// The original Content-* headers could be invalidated,
	// e.g. initial Content-Type could be 'application/json'.
	h := removeContentHeaders(req.Header().Clone())
	h.Set("Content-Type", "application/proto")
	out := &httpgrpc.HTTPRequest{
		Method:  http.MethodPost,
		Url:     url,
		Headers: connectHeaderToHTTPGRPCHeader(h),
	}
	var err error
	msg := req.Any()
	out.Body, err = proto.Marshal(msg.(proto.Message))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func removeContentHeaders(h http.Header) http.Header {
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "content-") {
			h.Del(k)
		}
	}
	return h
}

// filterHeader filters headers, which would expose details about the implementation details of the connectgrpc implementation
func filterHeader(name string) bool {
	if strings.ToLower(name) == "content-type" {
		return true
	}
	if strings.ToLower(name) == "accept-encoding" {
		return true
	}
	if strings.ToLower(name) == "content-encoding" {
		return true
	}
	return false
}

func decodeResponse[Resp any](r *httpgrpc.HTTPResponse) (*connect.Response[Resp], error) {
	if err := decompressResponse(r); err != nil {
		return nil, err
	}
	resp := &connect.Response[Resp]{Msg: new(Resp)}
	for _, h := range r.Headers {
		if filterHeader(h.Key) {
			continue
		}

		for _, v := range h.Values {
			resp.Header().Add(h.Key, v)
		}
	}
	if err := proto.Unmarshal(r.Body, resp.Any().(proto.Message)); err != nil {
		return nil, err
	}
	return resp, nil
}

func decompressResponse(r *httpgrpc.HTTPResponse) error {
	// We use gziphandler to compress responses of some methods,
	// therefore decompression is very likely to be required.
	// The handling is pretty much the same as in http.Transport,
	// which only supports gzip Content-Encoding.
	for _, h := range r.Headers {
		if h.Key == "Content-Encoding" {
			for _, v := range h.Values {
				switch {
				default:
					return fmt.Errorf("unsupported Content-Encoding: %s", v)
				case v == "":
				case strings.EqualFold(v, "gzip"):
					// bytes.Buffer implements flate.Reader, therefore
					// a gzip reader does not allocate a buffer.
					g, err := gzip.NewReader(bytes.NewBuffer(r.Body))
					if err != nil {
						return err
					}
					r.Body, err = io.ReadAll(g)
					return err
				}
			}
			return nil
		}
	}
	return nil
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

type responseWriter struct {
	header http.Header
	resp   httpgrpc.HTTPResponse
}

func (r *responseWriter) Header() http.Header {
	return r.header
}

func (r *responseWriter) Write(data []byte) (int, error) {
	r.resp.Body = append(r.resp.Body, data...)
	return len(data), nil
}

func (r *responseWriter) WriteHeader(statusCode int) {
	r.resp.Code = int32(statusCode)
}

func (r *responseWriter) HTTPResponse() *httpgrpc.HTTPResponse {
	r.resp.Headers = connectHeaderToHTTPGRPCHeader(r.header)
	return &r.resp
}

// NewHandler converts a Connect handler into a HTTPGRPC handler
type grpcHandler struct {
	next http.Handler
}

func NewHandler(h http.Handler) GRPCHandler {
	return &grpcHandler{next: h}
}

func newResponseWriter() *responseWriter {
	rw := &responseWriter{header: http.Header{}}
	rw.resp.Code = 200
	return rw
}

func (q *grpcHandler) Handle(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
	stdReq, err := http.NewRequestWithContext(ctx, req.Method, req.Url, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	stdReq.Header = httpgrpcHeaderToConnectHeader(req.Headers)

	rw := newResponseWriter()
	q.next.ServeHTTP(rw, stdReq)

	return rw.HTTPResponse(), nil
}

type httpgrpcClient struct {
	transport GRPCRoundTripper
}

func NewClient(transport GRPCRoundTripper) connect.HTTPClient {
	return &httpgrpcClient{transport: transport}
}

func (g *httpgrpcClient) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	resp, err := g.transport.RoundTripGRPC(req.Context(), &httpgrpc.HTTPRequest{
		Url:     req.URL.String(),
		Headers: connectHeaderToHTTPGRPCHeader(req.Header),
		Method:  req.Method,
		Body:    body,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc roundtripper error: %w", err)
	}

	return &http.Response{
		Body:          io.NopCloser(bytes.NewReader(resp.Body)),
		ContentLength: int64(len(resp.Body)),
		StatusCode:    int(resp.Code),
		Header:        httpgrpcHeaderToConnectHeader(resp.Headers),
	}, nil
}
