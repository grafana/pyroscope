package read_path

import (
	"context"

	"connectrpc.com/connect"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func (q *QueryBackend) ProfileTypes(
	ctx context.Context,
	c *connect.Request[querierv1.ProfileTypesRequest],
) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String())
	// DEPRECATED: This method is deprecated and will be removed in the future.
	return connect.NewResponse(&querierv1.ProfileTypesResponse{}), nil
}
