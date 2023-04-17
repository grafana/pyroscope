package querier

import (
	"github.com/bufbuild/connect-go"

	"github.com/grafana/phlare/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

func NewGRPCRoundTripper(transport connectgrpc.GRPCRoundTripper) querierv1connect.QuerierServiceHandler {
	return querierv1connect.NewQuerierServiceClient(
		connectgrpc.NewClient(transport),
		"http://httpgrpc",
		connect.WithGRPCWeb(),
	)
}
