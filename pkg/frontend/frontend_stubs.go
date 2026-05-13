package frontend

import (
	"context"

	"connectrpc.com/connect"

	"github.com/pkg/errors"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

var errNotAvailableInV1Frontend = connect.NewError(connect.CodeUnimplemented, errors.New("This endpoint is not available in v1 frontend"))

func (f *Frontend) SelectHeatmap(
	ctx context.Context,
	c *connect.Request[querierv1.SelectHeatmapRequest],
) (*connect.Response[querierv1.SelectHeatmapResponse], error) {
	return nil, errNotAvailableInV1Frontend
}

func (f *Frontend) SelectMergeStacktracesStream(
	_ context.Context,
	_ *connect.Request[querierv1.SelectMergeStacktracesRequest],
	_ *connect.ServerStream[querierv1.SelectMergeStacktracesPartial],
) error {
	return errNotAvailableInV1Frontend
}

func (f *Frontend) SelectSeriesStream(
	_ context.Context,
	_ *connect.Request[querierv1.SelectSeriesRequest],
	_ *connect.ServerStream[querierv1.SelectSeriesPartial],
) error {
	return errNotAvailableInV1Frontend
}
