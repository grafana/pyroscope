package query_frontend

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// TODO(kolesnikovae): Decide whether we want to implement those.

func (q *QueryFrontend) AnalyzeQuery(
	context.Context,
	*connect.Request[querierv1.AnalyzeQueryRequest],
) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	return connect.NewResponse(&querierv1.AnalyzeQueryResponse{}), nil
}

func (q *QueryFrontend) GetProfileStats(
	context.Context,
	*connect.Request[typesv1.GetProfileStatsRequest],
) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	return connect.NewResponse(&typesv1.GetProfileStatsResponse{}), nil
}
