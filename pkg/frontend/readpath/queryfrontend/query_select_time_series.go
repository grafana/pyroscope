package queryfrontend

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/pkg/errors"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
	"github.com/grafana/pyroscope/pkg/model/timeseries"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) SelectSeries(
	ctx context.Context,
	c *connect.Request[querierv1.SelectSeriesRequest],
) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.SelectSeriesResponse{}), nil
	}

	_, err = phlaremodel.ParseProfileTypeSelector(c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if c.Msg.Step == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("step must be non-zero"))
	}

	stepMs := time.Duration(c.Msg.Step * float64(time.Second)).Milliseconds()
	start := c.Msg.Start - stepMs

	labelSelector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// TODO: Once queryCompact is fully rolled out, use it for all queries and remove queryStandard.
	var series []*typesv1.Series
	if c.Msg.GetExemplarType() == typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL {
		series, err = q.queryCompact(ctx, start, c.Msg.End, labelSelector, c.Msg)
	} else {
		series, err = q.queryStandard(ctx, start, c.Msg.End, labelSelector, c.Msg)
	}
	if err != nil {
		return nil, err
	}

	series = timeseries.TopSeries(series, int(c.Msg.GetLimit()))
	return connect.NewResponse(&querierv1.SelectSeriesResponse{Series: series}), nil
}

func (q *QueryFrontend) queryStandard(ctx context.Context, start, end int64, labelSelector string, req *querierv1.SelectSeriesRequest) ([]*typesv1.Series, error) {
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       end,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         req.GetStep(),
				GroupBy:      req.GetGroupBy(),
				Limit:        req.GetLimit(),
				ExemplarType: req.GetExemplarType(),
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil || report.TimeSeries == nil {
		return nil, nil
	}
	return report.TimeSeries.TimeSeries, nil
}

// queryCompact uses the compact time series format with attribute table interning.
// Currently only used for exemplar retrieval (EXEMPLAR_TYPE_INDIVIDUAL).
// The legacy queryStandard path is used for all other time series queries.
// TODO: Migrate all queries to use queryCompact and remove queryStandard.
func (q *QueryFrontend) queryCompact(ctx context.Context, start, end int64, labelSelector string, req *querierv1.SelectSeriesRequest) ([]*typesv1.Series, error) {
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       end,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES_COMPACT,
			TimeSeriesCompact: &queryv1.TimeSeriesQuery{
				Step:         req.GetStep(),
				GroupBy:      req.GetGroupBy(),
				Limit:        req.GetLimit(),
				ExemplarType: req.GetExemplarType(),
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil || report.TimeSeriesCompact == nil {
		return nil, nil
	}

	return expandQuerySeries(report.TimeSeriesCompact.TimeSeries, report.TimeSeriesCompact.AttributeTable), nil
}

func expandQuerySeries(series []*queryv1.Series, table *queryv1.AttributeTable) []*typesv1.Series {
	if len(series) == 0 {
		return nil
	}
	if table == nil {
		table = &queryv1.AttributeTable{}
	}

	result := make([]*typesv1.Series, len(series))
	for i, s := range series {
		points := make([]*typesv1.Point, len(s.Points))
		for j, p := range s.Points {
			points[j] = &typesv1.Point{Value: p.Value, Timestamp: p.Timestamp}
			if len(p.AnnotationRefs) > 0 {
				points[j].Annotations = attributetable.ResolveAnnotations(p.AnnotationRefs, table)
			}
			if len(p.Exemplars) > 0 {
				points[j].Exemplars = attributetable.ResolveExemplars(p.Exemplars, table)
			}
		}
		result[i] = &typesv1.Series{
			Labels: attributetable.ResolveLabelPairs(s.AttributeRefs, table),
			Points: points,
		}
	}
	return result
}
