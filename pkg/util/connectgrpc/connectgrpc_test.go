package connectgrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/grafana/phlare/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
)

type fakeQuerier struct {
	querierv1connect.UnimplementedQuerierServiceHandler
	req  *connect.Request[typesv1.LabelValuesRequest]
	resp *connect.Response[typesv1.LabelValuesResponse]
}

func (f *fakeQuerier) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	f.req = req
	return f.resp, nil
}

func Test_DecodeGRPC(t *testing.T) {
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
	_, _ = client.LabelValues(context.Background(), connect.NewRequest(req))

	encoded, err := encodeRequest(f.req)
	require.NoError(t, err)
	require.Equal(t, "POST", encoded.Method)
	require.Equal(t, "/querier.v1.QuerierService/LabelValues", encoded.Url)
	require.Len(t, encoded.Headers, 4)

	decoded, err := decodeRequest[typesv1.LabelValuesRequest](encoded)
	require.NoError(t, err)
	require.Equal(t, req.Name, decoded.Msg.Name)
}
