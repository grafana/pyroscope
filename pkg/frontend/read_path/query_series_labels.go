package read_path

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryBackend) Series(
	ctx context.Context,
	c *connect.Request[querierv1.SeriesRequest],
) (*connect.Response[querierv1.SeriesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("matchers", c.Msg.Matchers).
		SetTag("label_names", c.Msg.LabelNames)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.ValidateTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}

	if isProfileTypeQuery(c.Msg.LabelNames, c.Msg.Matchers) {
		_ = level.Debug(q.logger).Log("msg", "listing profile types from metadata as series labels")
		return listProfileTypesFromMetadataAsSeriesLabels(ctx, q.metastore, tenantIDs, c.Msg.Start, c.Msg.End, c.Msg.LabelNames)
	}

	labelSelector, err := buildLabelSelectorFromMatchers(c.Msg.Matchers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := q.Query(ctx, c.Msg.Start, c.Msg.End, tenantIDs, labelSelector, &querybackendv1.Query{
		QueryType: querybackendv1.QueryType_QUERY_SERIES_LABELS,
		SeriesLabels: &querybackendv1.SeriesLabelsQuery{
			LabelNames: c.Msg.LabelNames,
		},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}
	return connect.NewResponse(&querierv1.SeriesResponse{LabelsSet: report.SeriesLabels.SeriesLabels}), nil
}
