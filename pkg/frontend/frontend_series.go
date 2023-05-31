package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

func (f *Frontend) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.SeriesRequest, querierv1.SeriesResponse](ctx, f, c)
}
