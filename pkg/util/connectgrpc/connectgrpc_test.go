package connectgrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/util/httpgrpc"
)

type fakeQuerier struct {
	querierv1connect.UnimplementedQuerierServiceHandler
	req  *connect.Request[typesv1.LabelValuesRequest]
	resp *connect.Response[typesv1.LabelValuesResponse]
}

func (f *fakeQuerier) LabelValues(_ context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	f.req = req
	return f.resp, nil
}

type mockRoundTripper struct {
	req  *httpgrpc.HTTPRequest
	resp *httpgrpc.HTTPResponse
}

func (m *mockRoundTripper) RoundTripGRPC(_ context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
	m.req = req
	return m.resp, nil
}

func Test_RoundTripUnary(t *testing.T) {
	request := func(t *testing.T) *connect.Request[typesv1.LabelValuesRequest] {
		server := httptest.NewUnstartedServer(nil)
		mux := mux.NewRouter()
		server.Config.Handler = h2c.NewHandler(mux, &http2.Server{})

		server.Start()
		defer server.Close()
		f := &fakeQuerier{resp: &connect.Response[typesv1.LabelValuesResponse]{
			Msg: &typesv1.LabelValuesResponse{Names: []string{"foo", "bar"}},
		}}
		querierv1connect.RegisterQuerierServiceHandler(mux, f)

		client := querierv1connect.NewQuerierServiceClient(http.DefaultClient, server.URL)
		req := &typesv1.LabelValuesRequest{
			Name: "foo",
		}
		_, err := client.LabelValues(context.Background(), connect.NewRequest(req))
		require.NoError(t, err)
		return f.req
	}

	t.Run("HTTP request can trip GRPC", func(t *testing.T) {
		req := request(t)
		m := &mockRoundTripper{resp: &httpgrpc.HTTPResponse{Code: 200}}
		_, err := RoundTripUnary[typesv1.LabelValuesRequest, typesv1.LabelValuesResponse](context.Background(), m, req)
		require.NoError(t, err)
		require.Equal(t, "POST", m.req.Method)
		require.Equal(t, "/querier.v1.QuerierService/LabelValues", m.req.Url)
		actualHeaders := lo.Map(m.req.Headers, func(h *httpgrpc.Header, index int) string {
			return h.Key + ": " + strings.Join(h.Values, ",")
		})
		require.Contains(t, actualHeaders, "Content-Type: application/proto")
		require.Contains(t, actualHeaders, "Connect-Protocol-Version: 1")
		require.Contains(t, actualHeaders, "Accept-Encoding: gzip")

		decoded, err := decodeRequest[typesv1.LabelValuesRequest](m.req)
		require.NoError(t, err)
		require.Equal(t, req.Msg.Name, decoded.Msg.Name)
	})

	t.Run("HTTP request URL can be overridden", func(t *testing.T) {
		req := request(t)
		m := &mockRoundTripper{resp: &httpgrpc.HTTPResponse{Code: 200}}
		const url = "TestURL"
		ctx := WithProcedure(context.Background(), url)
		_, err := RoundTripUnary[typesv1.LabelValuesRequest, typesv1.LabelValuesResponse](ctx, m, req)
		require.NoError(t, err)
		require.Equal(t, url, m.req.Url)
	})
}
