package queryfrontend

import (
	"context"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/frontend/readpath/queryfrontend/diagnostics"
)

// saveDiagnostics saves query diagnostics and sets response headers if diagnostics collection is enabled.
// It extracts all necessary information from the diagnostics object, including tenant IDs from context
// and response time from the injected start time.
// This is a best-effort operation - errors are logged but not returned.
func (q *QueryFrontend) saveDiagnostics(
	ctx context.Context,
	diag *queryv1.Diagnostics,
	respHeaders map[string][]string,
) {
	if diag == nil {
		return
	}

	storedQuery := buildStoredQueryFromDiagnostics(diag)

	// Calculate response time from the start time injected when the request was received.
	if startTime := diagnostics.QueryStartTime(ctx); !startTime.IsZero() {
		storedQuery.ResponseTimeMs = time.Since(startTime).Milliseconds()
	}

	tenantIDs, _ := tenant.TenantIDs(ctx)
	var diagID string
	if q.diagnosticsStore != nil && len(tenantIDs) > 0 {
		var err error
		diagID, err = q.diagnosticsStore.Save(ctx, tenantIDs[0], storedQuery, diag.QueryPlan, diag.ExecutionNode)
		if err != nil {
			level.Warn(q.logger).Log("msg", "failed to save diagnostics", "err", err)
		}
	} else if q.diagnosticsStore == nil {
		level.Debug(q.logger).Log("msg", "diagnostics store not configured, skipping save")
	}
	diagnostics.SetHeader(respHeaders, diagID)
}

// buildStoredQueryFromDiagnostics constructs a StoredQuery from the QueryRequest in diagnostics.
func buildStoredQueryFromDiagnostics(diag *queryv1.Diagnostics) *diagnostics.StoredQuery {
	req := diag.QueryRequest
	if req == nil {
		return &diagnostics.StoredQuery{}
	}

	sq := &diagnostics.StoredQuery{
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		LabelSelector: req.LabelSelector,
	}

	if len(req.Query) > 0 {
		query := req.Query[0]
		sq.QueryType = query.QueryType.String()

		switch query.QueryType {
		case queryv1.QueryType_QUERY_LABEL_NAMES:
			// No additional parameters
		case queryv1.QueryType_QUERY_LABEL_VALUES:
			if query.LabelValues != nil {
				sq.LabelName = query.LabelValues.LabelName
			}
		case queryv1.QueryType_QUERY_SERIES_LABELS:
			if query.SeriesLabels != nil {
				sq.SeriesLabelNames = query.SeriesLabels.LabelNames
			}
		case queryv1.QueryType_QUERY_TIME_SERIES:
			if query.TimeSeries != nil {
				sq.Step = query.TimeSeries.Step
				sq.GroupBy = query.TimeSeries.GroupBy
				sq.Limit = int64(query.TimeSeries.Limit)
			}
		case queryv1.QueryType_QUERY_TREE:
			if query.Tree != nil {
				sq.MaxNodes = query.Tree.MaxNodes
			}
		case queryv1.QueryType_QUERY_PPROF:
			if query.Pprof != nil {
				sq.MaxNodes = query.Pprof.MaxNodes
			}
		case queryv1.QueryType_QUERY_HEATMAP:
			if query.Heatmap != nil {
				sq.Step = query.Heatmap.Step
				sq.GroupBy = query.Heatmap.GroupBy
				sq.Limit = query.Heatmap.GetLimit()
			}
		}
	}

	return sq
}

func countBlocksRead(node *queryv1.ExecutionNode) int64 {
	if node == nil {
		return 0
	}
	var total int64
	if node.Stats != nil {
		total += node.Stats.BlocksRead
	}
	for _, child := range node.Children {
		total += countBlocksRead(child)
	}
	return total
}
