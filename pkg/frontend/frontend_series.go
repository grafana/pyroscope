package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func (f *Frontend) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSeriesProcedure)
	return connectgrpc.RoundTripUnary[querierv1.SeriesRequest, querierv1.SeriesResponse](ctx, f, c)
}
