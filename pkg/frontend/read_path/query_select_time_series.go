package read_path

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryBackend) SelectSeries(
	ctx context.Context,
	c *connect.Request[querierv1.SelectSeriesRequest],
) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("step", c.Msg.Step).
		SetTag("by", c.Msg.GroupBy).
		SetTag("profile_type", c.Msg.ProfileTypeID)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.ValidateTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.SelectSeriesResponse{}), nil
	}

	labelSelector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, err
	}
	report, err := q.Query(ctx, c.Msg.Start, c.Msg.End, tenantIDs, labelSelector, &querybackendv1.Query{
		QueryType: querybackendv1.QueryType_QUERY_TIME_SERIES,
		TimeSeries: &querybackendv1.TimeSeriesQuery{
			Step:        c.Msg.GetStep(),
			GroupBy:     c.Msg.GetGroupBy(),
			Aggregation: c.Msg.Aggregation,
		},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&querierv1.SelectSeriesResponse{}), nil
	}
	return connect.NewResponse(&querierv1.SelectSeriesResponse{Series: report.TimeSeries.TimeSeries}), nil
}