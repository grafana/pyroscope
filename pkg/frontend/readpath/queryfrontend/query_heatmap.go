package queryfrontend

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/heatmap"
	"github.com/grafana/pyroscope/pkg/validation"
)

const (
	// stepAdjustment is subtracted from the start time to ensure we capture
	// data points that fall before the first time bucket boundary
	stepAdjustment = 1
)

func (q *QueryFrontend) SelectHeatmap(
	ctx context.Context,
	c *connect.Request[querierv1.SelectHeatmapRequest],
) (*connect.Response[querierv1.SelectHeatmapResponse], error) {
	// Validate step
	if c.Msg.Step <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("step must be greater than 0, got %f", c.Msg.Step))
	}

	// Validate limit if provided
	if c.Msg.Limit != nil && *c.Msg.Limit < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("limit must be non-negative, got %d", *c.Msg.Limit))
	}

	// Validate time range
	if c.Msg.Start >= c.Msg.End {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("start time must be before end time, got start=%d end=%d", c.Msg.Start, c.Msg.End))
	}

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
	start := c.Msg.Start - (stepMs * stepAdjustment)

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
				Step:         c.Msg.GetStep(),
				GroupBy:      c.Msg.GetGroupBy(),
				QueryType:    c.Msg.QueryType,
				ExemplarType: c.Msg.GetExemplarType(),
				Limit:        c.Msg.GetLimit(),
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
		c.Msg.GetGroupBy(),
		c.Msg.GetExemplarType(),
	)

	// Apply limit if specified
	if c.Msg.GetLimit() > 0 {
		series = heatmap.TopSeries(series, int(c.Msg.GetLimit()))
	}

	return connect.NewResponse(&querierv1.SelectHeatmapResponse{
		Series: series,
	}), nil
}
