package frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/opentracing/opentracing-go"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func (f *Frontend) AnalyzeQuery(ctx context.Context,
	c *connect.Request[querierv1.AnalyzeQueryRequest]) (
	*connect.Response[querierv1.AnalyzeQueryResponse], error,
) {
	opentracing.SpanFromContext(ctx)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceAnalyzeQueryProcedure)
	res, err := connectgrpc.RoundTripUnary[querierv1.AnalyzeQueryRequest, querierv1.AnalyzeQueryResponse](ctx, f, c)
	return res, err
}
