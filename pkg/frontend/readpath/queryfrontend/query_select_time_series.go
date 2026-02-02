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

	labelMap := make(map[int64]*typesv1.LabelPair, len(table.Keys))
	for i := range table.Keys {
		labelMap[int64(i)] = &typesv1.LabelPair{Name: table.Keys[i], Value: table.Values[i]}
	}

	result := make([]*typesv1.Series, len(series))
	for i, s := range series {
		labels := make([]*typesv1.LabelPair, len(s.AttributeRefs))
		for j, ref := range s.AttributeRefs {
			labels[j] = labelMap[ref]
		}

		points := make([]*typesv1.Point, len(s.Points))
		for j, p := range s.Points {
			points[j] = &typesv1.Point{Value: p.Value, Timestamp: p.Timestamp}
			if len(p.AnnotationRefs) > 0 {
				points[j].Annotations = make([]*typesv1.ProfileAnnotation, len(p.AnnotationRefs))
				for k, ref := range p.AnnotationRefs {
					kv := labelMap[ref]
					points[j].Annotations[k] = &typesv1.ProfileAnnotation{Key: kv.Name, Value: kv.Value}
				}
			}
			if len(p.Exemplars) > 0 {
				points[j].Exemplars = make([]*typesv1.Exemplar, len(p.Exemplars))
				for k, ex := range p.Exemplars {
					exLabels := make([]*typesv1.LabelPair, len(ex.AttributeRefs))
					for l, ref := range ex.AttributeRefs {
						exLabels[l] = labelMap[ref]
					}
					points[j].Exemplars[k] = &typesv1.Exemplar{
						Timestamp: ex.Timestamp,
						ProfileId: ex.ProfileId,
						SpanId:    ex.SpanId,
						Value:     ex.Value,
						Labels:    exLabels,
					}
				}
			}
		}

		result[i] = &typesv1.Series{Labels: labels, Points: points}
	}

	return result
}
