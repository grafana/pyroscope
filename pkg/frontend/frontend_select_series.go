package frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) SelectSeries(ctx context.Context,
	c *connect.Request[querierv1.SelectSeriesRequest]) (
	*connect.Response[querierv1.SelectSeriesResponse], error,
) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("step", c.Msg.Step).
		SetTag("by", c.Msg.GroupBy).
		SetTag("profile_type", c.Msg.ProfileTypeID)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSelectSeriesProcedure)
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, model.Interval{Start: model.Time(c.Msg.Start), End: model.Time(c.Msg.End)}, model.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if validated.IsEmpty {
		return connect.NewResponse(&querierv1.SelectSeriesResponse{}), nil
	}
	c.Msg.Start = int64(validated.Start)
	c.Msg.End = int64(validated.End)

	query, err := buildLabelSelectorAndProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if _, err = f.listMetadata(ctx, tenantIDs, c.Msg.Start, c.Msg.End, query); err != nil {
		return nil, err
	}
	// TODO: Call query-backend.
	return connect.NewResponse(&querierv1.SelectSeriesResponse{}), nil
}
