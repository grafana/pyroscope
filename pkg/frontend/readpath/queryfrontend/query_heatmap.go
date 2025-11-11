package queryfrontend

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/heatmap"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) SelectHeatmap(
	ctx context.Context,
	c *connect.Request[querierv1.SelectHeatmapRequest],
) (*connect.Response[querierv1.SelectHeatmapResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.SelectHeatmapResponse{}), nil
	}

	_, err = phlaremodel.ParseProfileTypeSelector(c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	stepMs := time.Duration(c.Msg.Step * float64(time.Second)).Milliseconds()
	start := c.Msg.Start - stepMs

	labelSelector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, err
	}
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       c.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_HEATMAP,
			Heatmap: &queryv1.HeatmapQuery{
				Step:      c.Msg.GetStep(),
				GroupBy:   c.Msg.GetGroupBy(),
				QueryType: c.Msg.QueryType,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil || report.Heatmap == nil {
		return connect.NewResponse(&querierv1.SelectHeatmapResponse{}), nil
	}
	// Convert HeatmapReport to HeatmapSeries using RangeHeatmap
	series := heatmap.RangeHeatmap(
		[]*queryv1.HeatmapReport{report.Heatmap},
		start,
		c.Msg.End,
		stepMs,
		nil,
	)
	return connect.NewResponse(&querierv1.SelectHeatmapResponse{
		Series: series,
	}), nil
}