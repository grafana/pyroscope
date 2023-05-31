package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

func (f *Frontend) ProfileTypes(ctx context.Context, c *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.ProfileTypesRequest, querierv1.ProfileTypesResponse](ctx, f, c)
}
