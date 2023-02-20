package querier

import (
	"context"
	"net/http"

	"github.com/grafana/phlare/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
	"github.com/grafana/phlare/pkg/util/httpgrpc"
)

// todo: this could be generated but first we need more operational experience in case we need to change it.
type grpcHandler struct {
	querierv1connect.QuerierServiceHandler
}

func NewGRPCHandler(svc querierv1connect.QuerierServiceHandler) connectgrpc.GRPCHandler {
	return &grpcHandler{svc}
}

func (q *grpcHandler) Handle(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
	switch req.Url {
	case "/querier.v1.QuerierService/ProfileTypes":
		return connectgrpc.HandleUnary(ctx, req, q.ProfileTypes)
	case "/querier.v1.QuerierService/LabelValues":
		return connectgrpc.HandleUnary(ctx, req, q.LabelValues)
	case "/querier.v1.QuerierService/LabelNames":
		return connectgrpc.HandleUnary(ctx, req, q.LabelNames)
	case "/querier.v1.QuerierService/Series":
		return connectgrpc.HandleUnary(ctx, req, q.Series)
	case "/querier.v1.QuerierService/SelectMergeStacktraces":
		return connectgrpc.HandleUnary(ctx, req, q.SelectMergeStacktraces)
	case "/querier.v1.QuerierService/SelectMergeProfile":
		return connectgrpc.HandleUnary(ctx, req, q.SelectMergeProfile)
	case "/querier.v1.QuerierService/SelectSeries":
		return connectgrpc.HandleUnary(ctx, req, q.SelectSeries)
	default:
		return nil, httpgrpc.Errorf(http.StatusNotFound, "url %s not found", req.Url)
	}
}
