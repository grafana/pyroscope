package tenant

import (
	"context"
	"net/http"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/stretchr/testify/require"
)

func Test_AuthInterceptor(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
		"client: forward from context": func(t *testing.T) {
			i := NewAuthInterceptor(false)
			resp, err := i.WrapUnary(func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
				tenantID, _, err := ExtractTenantIDFromHeaders(context.Background(), ar.Header())
				require.NoError(t, err)
				require.Equal(t, tenantID, "foo")
				return nil, nil
			})(InjectTenantID(context.Background(), "foo"), newFakeReq(true))
			require.NoError(t, err)
			require.Nil(t, resp)
		},
		"client: no org forwarded": func(t *testing.T) {
			i := NewAuthInterceptor(false)
			resp, err := i.WrapUnary(func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
				tenantID, _, err := ExtractTenantIDFromHeaders(context.Background(), ar.Header())
				require.Equal(t, ErrNoTenantID, err)
				require.Equal(t, tenantID, "")
				return nil, nil
			})(context.Background(), newFakeReq(true))
			require.NoError(t, err)
			require.Nil(t, resp)
		},
		"server: disable, static org": func(t *testing.T) {
			i := NewAuthInterceptor(false)
			req := newFakeReq(false)
			req.Header().Set("X-Scope-OrgID", "foo")
			resp, err := i.WrapUnary(func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
				tenantID, err := ExtractTenantIDFromContext(ctx)
				require.NoError(t, err)
				require.Equal(t, tenantID, DefaultTenantID)
				return nil, nil
			})(context.Background(), req)
			require.NoError(t, err)
			require.Nil(t, resp)
		},
		"server: enable, forward header": func(t *testing.T) {
			i := NewAuthInterceptor(true)
			req := newFakeReq(false)
			req.Header().Set("X-Scope-OrgID", "foo")
			resp, err := i.WrapUnary(func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
				tenantID, err := ExtractTenantIDFromContext(ctx)
				require.NoError(t, err)
				require.Equal(t, tenantID, "foo")
				return nil, nil
			})(context.Background(), req)
			require.NoError(t, err)
			require.Nil(t, resp)
		},
		"streaming client should forward from context": func(t *testing.T) {
			i := NewAuthInterceptor(false)
			inConn := newFakeClientStreamingConn()
			outConn := i.WrapStreamingClient(func(ctx context.Context, s connect.Spec) connect.StreamingClientConn {
				return inConn
			})(InjectTenantID(context.Background(), "foo"), connect.Spec{})
			require.Equal(t, "foo", outConn.RequestHeader().Get("X-Scope-OrgID"))
		},
		"streaming server should forward from header to context if enabled": func(t *testing.T) {
			i := NewAuthInterceptor(true)
			shc := newFakeClientStreamingConn()
			shc.requestHeaders.Set("X-Scope-OrgID", "foo")
			_ = i.WrapStreamingHandler(func(ctx context.Context, shc connect.StreamingHandlerConn) error {
				tenantID, err := ExtractTenantIDFromContext(ctx)
				require.NoError(t, err)
				require.Equal(t, tenantID, "foo")
				return nil
			})(context.Background(), shc)
		},
		"streaming server should forward default tenant to context if disable": func(t *testing.T) {
			i := NewAuthInterceptor(false)
			shc := newFakeClientStreamingConn()
			_ = i.WrapStreamingHandler(func(ctx context.Context, shc connect.StreamingHandlerConn) error {
				tenantID, err := ExtractTenantIDFromContext(ctx)
				require.NoError(t, err)
				require.Equal(t, tenantID, DefaultTenantID)
				return nil
			})(context.Background(), shc)
		},
	} {
		t.Run(testName, testCase)
	}
}

type fakeReq struct {
	connect.AnyRequest
	isClient bool
	headers  http.Header
}

func newFakeReq(isClient bool) fakeReq {
	return fakeReq{
		isClient:   isClient,
		headers:    http.Header{},
		AnyRequest: connect.NewRequest(&http.Request{}),
	}
}

func (f fakeReq) Spec() connect.Spec {
	return connect.Spec{
		IsClient: f.isClient,
	}
}

func (f fakeReq) Header() http.Header {
	return f.headers
}

type fakeClientStreamingConn struct {
	requestHeaders http.Header
}

func newFakeClientStreamingConn() fakeClientStreamingConn {
	return fakeClientStreamingConn{
		requestHeaders: http.Header{},
	}
}

func (fakeClientStreamingConn) Spec() connect.Spec           { return connect.Spec{} }
func (fakeClientStreamingConn) Send(any) error               { return nil }
func (f fakeClientStreamingConn) RequestHeader() http.Header { return f.requestHeaders }
func (fakeClientStreamingConn) CloseRequest() error          { return nil }
func (fakeClientStreamingConn) Receive(any) error            { return nil }
func (fakeClientStreamingConn) ResponseHeader() http.Header  { return nil }
func (fakeClientStreamingConn) ResponseTrailer() http.Header { return nil }
func (fakeClientStreamingConn) CloseResponse() error         { return nil }
