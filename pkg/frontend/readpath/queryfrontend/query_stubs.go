package queryfrontend

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// TODO(kolesnikovae): Decide whether we want to implement those.

func (q *QueryFrontend) AnalyzeQuery(
	context.Context,
	*connect.Request[querierv1.AnalyzeQueryRequest],
) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	return connect.NewResponse(&querierv1.AnalyzeQueryResponse{}), nil
}
