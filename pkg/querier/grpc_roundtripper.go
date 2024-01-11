package querier

import (
	"connectrpc.com/connect"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func NewGRPCRoundTripper(transport connectgrpc.GRPCRoundTripper) querierv1connect.QuerierServiceHandler {
	return querierv1connect.NewQuerierServiceClient(
		connectgrpc.NewClient(transport),
		"http://httpgrpc",
		connect.WithGRPCWeb(),
	)
}
