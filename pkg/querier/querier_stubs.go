package querier

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// SelectMergeStacktracesStream and SelectSeriesStream are not implemented on the
// V1 querier. The V2 streaming path is handled by the query frontend.

func (q *Querier) SelectMergeStacktracesStream(
	_ context.Context,
	_ *connect.Request[querierv1.SelectMergeStacktracesRequest],
	_ *connect.ServerStream[querierv1.SelectMergeStacktracesPartial],
) error {
	return status.Error(codes.Unimplemented, "streaming not supported on V1 path")
}

func (q *Querier) SelectSeriesStream(
	_ context.Context,
	_ *connect.Request[querierv1.SelectSeriesRequest],
	_ *connect.ServerStream[querierv1.SelectSeriesPartial],
) error {
	return status.Error(codes.Unimplemented, "streaming not supported on V1 path")
}
