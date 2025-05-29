package frontend

import (
	"context"

	"connectrpc.com/connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func (f *Frontend) GetFeatureFlags(ctx context.Context,
	req *connect.Request[typesv1.GetFeatureFlagsRequest],
) (*connect.Response[typesv1.GetFeatureFlagsResponse], error) {
	return connectgrpc.RoundTripUnary[typesv1.GetFeatureFlagsRequest, typesv1.GetFeatureFlagsResponse](ctx, f, req)
}
