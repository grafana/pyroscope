package queryfrontend

import (
	"context"
	"fmt"
	"math"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model/timeseries"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func (q *QueryFrontend) queryTimeSeriesAnomalies(ctx context.Context, req *querierv1.QueryAnomaliesRequest) ([]*querierv1.TimeSeriesAnomaly, error) {
	options := req.TimeSeries
	if options == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("time_series is required for ANOMALY_TYPE_TIME_SERIES"))
	}
	if math.IsNaN(options.Step) || math.IsInf(options.Step, 0) || options.Step < timeseries.MinimumStep.Seconds() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("step must be >= 1ms and finite"))
	}
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	start, end := req.Start, req.End
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &start, &end)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return nil, nil
	}
	labelSelector, err := buildLabelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID)
	if err != nil {
		return nil, err
	}
	stepMillis := time.Duration(options.Step * float64(time.Second)).Milliseconds()
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     start - stepMillis,
		EndTime:       end,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType:          queryv1.QueryType_QUERY_TIME_SERIES_ANALYSIS,
			TimeSeriesAnalysis: &queryv1.TimeSeriesAnalysisQuery{Step: options.Step, GroupBy: options.GroupBy, Limit: options.GetLimit()},
		}},
	}, nil)
	if err != nil {
		return nil, err
	}
	if report == nil || report.TimeSeriesAnalysis == nil {
		return nil, nil
	}
	return report.TimeSeriesAnalysis.Events, nil
}
