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

	exemplarType := c.Msg.GetExemplarType()
	var series []*typesv1.Series
	if exemplarType == typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL {
		series, err = q.queryWithAttributeTable(ctx, start, c.Msg.End, labelSelector, c.Msg)
	} else {
		series, err = q.query(ctx, start, c.Msg.End, labelSelector, c.Msg)
	}
	if err != nil {
		return nil, err
	}

	series = phlaremodel.TopSeries(series, int(c.Msg.GetLimit()))
	return connect.NewResponse(&querierv1.SelectSeriesResponse{Series: series}), nil
}

// queryStandard queries using the standard path without exemplar optimization.
func (q *QueryFrontend) query(
	ctx context.Context,
	start, end int64,
	labelSelector string,
	req *querierv1.SelectSeriesRequest,
) ([]*typesv1.Series, error) {
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

// queryWithAttributeTable queries using the AttributeTable optimization for exemplars.
func (q *QueryFrontend) queryWithAttributeTable(
	ctx context.Context,
	start, end int64,
	labelSelector string,
	req *querierv1.SelectSeriesRequest,
) ([]*typesv1.Series, error) {
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       end,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES_WITH_ATTRIBUTE_TABLE,
			TimeSeriesWithAttributeTable: &queryv1.TimeSeriesQuery{
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
	if report == nil || report.TimeSeriesWithAttributeTable == nil {
		return nil, nil
	}

	return convertQuerySeriesToTypesSeries(
		report.TimeSeriesWithAttributeTable.TimeSeries,
		report.TimeSeriesWithAttributeTable.AttributeTable,
	), nil
}

// convertQuerySeriesToTypesSeries converts query.v1.Series (with attribute_refs) to types.v1.Series.
func convertQuerySeriesToTypesSeries(querySeries []*queryv1.Series, attrTable *queryv1.AttributeTable) []*typesv1.Series {
	if len(querySeries) == 0 {
		return nil
	}

	result := make([]*typesv1.Series, len(querySeries))
	for i, qs := range querySeries {
		points := make([]*typesv1.Point, len(qs.Points))
		for j, qp := range qs.Points {
			var exemplars []*typesv1.Exemplar
			if len(qp.Exemplars) > 0 {
				exemplars = make([]*typesv1.Exemplar, len(qp.Exemplars))
				for k, qex := range qp.Exemplars {
					labels := expandAttributeRefs(qex.AttributeRefs, attrTable)
					exemplars[k] = &typesv1.Exemplar{
						Timestamp: qex.Timestamp,
						ProfileId: qex.ProfileId,
						SpanId:    qex.SpanId,
						Value:     qex.Value,
						Labels:    labels,
					}
				}
			}

			points[j] = &typesv1.Point{
				Value:       qp.Value,
				Timestamp:   qp.Timestamp,
				Annotations: qp.Annotations,
				Exemplars:   exemplars,
			}
		}

		// Expand series attribute_refs back to labels
		seriesLabels := expandAttributeRefs(qs.AttributeRefs, attrTable)

		result[i] = &typesv1.Series{
			Labels: seriesLabels,
			Points: points,
		}
	}

	return result
}

// expandAttributeRefs converts attribute_refs indices back to label pairs.
func expandAttributeRefs(refs []int64, attrTable *queryv1.AttributeTable) []*typesv1.LabelPair {
	if len(refs) == 0 || attrTable == nil {
		return nil
	}

	labels := make([]*typesv1.LabelPair, len(refs))
	for i, ref := range refs {
		if ref >= 0 && ref < int64(len(attrTable.Keys)) {
			labels[i] = &typesv1.LabelPair{
				Name:  attrTable.Keys[ref],
				Value: attrTable.Values[ref],
			}
		}
	}

	return labels
}
