package query_frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) Series(
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
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}

	if q.isProfileTypeQuery(c.Msg.LabelNames, c.Msg.Matchers) {
		level.Debug(q.logger).Log("msg", "listing profile types from metadata as series labels")
		return q.queryProfileTypeMetadataLabels(ctx, tenantIDs, c.Msg.Start, c.Msg.End, c.Msg.LabelNames)
	}

	labelSelector, err := buildLabelSelectorFromMatchers(c.Msg.Matchers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     c.Msg.Start,
		EndTime:       c.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{
				LabelNames: c.Msg.LabelNames,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}
	return connect.NewResponse(&querierv1.SeriesResponse{LabelsSet: report.SeriesLabels.SeriesLabels}), nil
}
