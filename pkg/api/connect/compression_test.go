package connectapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

func TestGzipRoundTripProtocols(t *testing.T) {
	const procedure = "/test.Profiles/Echo"
	handler := connect.NewUnaryHandler(procedure,
		func(_ context.Context, req *connect.Request[profilev1.Profile]) (*connect.Response[profilev1.Profile], error) {
			return connect.NewResponse(req.Msg), nil
		}, WithGzipHandler(), connect.WithCodec(ProtoCodec), connect.WithCompressMinBytes(0))
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, protocol := range []struct {
		name string
		opts []connect.ClientOption
	}{
		{name: "connect"},
		{name: "grpc", opts: []connect.ClientOption{connect.WithGRPC()}},
		{name: "grpc-web", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			opts := []connect.ClientOption{WithGzipClient(), connect.WithCodec(ProtoCodec), connect.WithSendCompression(compressionGzip), connect.WithCompressMinBytes(0)}
			opts = append(opts, protocol.opts...)
			client := connect.NewClient[profilev1.Profile, profilev1.Profile](server.Client(), server.URL+procedure, opts...)
			for _, size := range []int{100, 3 << 20, 100, 3 << 20} {
				p := &profilev1.Profile{StringTable: []string{"", strings.Repeat("x", size)}}
				response, err := client.CallUnary(context.Background(), connect.NewRequest(p))
				require.NoError(t, err)
				require.True(t, proto.Equal(p, response.Msg))
			}
		})
	}
}
