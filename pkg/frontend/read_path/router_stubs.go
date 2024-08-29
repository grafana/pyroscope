package read_path

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func (r *Router) AnalyzeQuery(
	context.Context,
	*connect.Request[querierv1.AnalyzeQueryRequest],
) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	return connect.NewResponse(&querierv1.AnalyzeQueryResponse{}), nil
}

func (r *Router) GetProfileStats(
	context.Context,
	*connect.Request[typesv1.GetProfileStatsRequest],
) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	return connect.NewResponse(&typesv1.GetProfileStatsResponse{}), nil
}

func (r *Router) ProfileTypes(
	context.Context,
	*connect.Request[querierv1.ProfileTypesRequest],
) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	// DEPRECATED: This method is deprecated and will be removed in the future.
	return connect.NewResponse(&querierv1.ProfileTypesResponse{}), nil
}
