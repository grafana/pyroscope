package frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/opentracing/opentracing-go"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func (f *Frontend) GetProfileStats(ctx context.Context,
	c *connect.Request[typesv1.GetProfileStatsRequest]) (
	*connect.Response[typesv1.GetProfileStatsResponse], error,
) {
	opentracing.SpanFromContext(ctx)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceGetProfileStatsProcedure)
	res, err := connectgrpc.RoundTripUnary[typesv1.GetProfileStatsRequest, typesv1.GetProfileStatsResponse](ctx, f, c)
	return res, err
}
