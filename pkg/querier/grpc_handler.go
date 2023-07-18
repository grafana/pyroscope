package querier

import (
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func NewGRPCHandler(svc querierv1connect.QuerierServiceHandler) connectgrpc.GRPCHandler {
	_, h := querierv1connect.NewQuerierServiceHandler(svc)
	return connectgrpc.NewHandler(h)
}
