package read_path

import (
	"context"

	"connectrpc.com/connect"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// TODO(kolesnikovae): Implement.

func (q *Router) GetProfileStats(
	context.Context,
	*connect.Request[typesv1.GetProfileStatsRequest],
) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	return connect.NewResponse(&typesv1.GetProfileStatsResponse{}), nil
}
