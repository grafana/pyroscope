package query_frontend

import (
	"context"

	"connectrpc.com/connect"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func (q *QueryFrontend) GetFeatureFlags(
	ctx context.Context,
	req *connect.Request[typesv1.GetFeatureFlagsRequest],
) (*connect.Response[typesv1.GetFeatureFlagsResponse], error) {
	return q.featureFlags.GetFeatureFlags(ctx, req)
}
