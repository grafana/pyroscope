package querier

import (
	"github.com/grafana/phlare/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

func NewGRPCHandler(svc querierv1connect.QuerierServiceHandler) connectgrpc.GRPCHandler {
	_, h := querierv1connect.NewQuerierServiceHandler(svc)
	return connectgrpc.NewHandler(h)
}
