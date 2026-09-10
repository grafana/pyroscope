package queryfrontend

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func (q *QueryFrontend) selectMergeStacktracesFunctions(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Functions: new(querierv1.FunctionTable)}), nil
	}
	maxNodes, err := validation.ValidateMaxNodes(q.limits, tenantIDs, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	table, err := q.queryFunctionTable(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	model.LimitFunctionTable(table, maxNodes)
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Functions: table}), nil
}

// queryFunctionTable returns all functions for a validated time range. Callers
// apply row limits only after all aggregation and any cross-profile join.
func (q *QueryFrontend) queryFunctionTable(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest) (*querierv1.FunctionTable, error) {
	if _, err := model.ParseProfileTypeSelector(req.ProfileTypeID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	selector, err := buildLabelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	// TODO: Native symbolization for function tables is deliberately omitted for now.
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime: req.Start, EndTime: req.End, LabelSelector: selector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_FUNCTIONS,
			Functions: &queryv1.FunctionsQuery{
				SpanSelector:       req.SpanSelector,
				StackTraceSelector: req.StackTraceSelector,
				ProfileIdSelector:  req.ProfileIdSelector,
				TraceIdSelector:    req.TraceIdSelector,
			},
		}},
	}, nil)
	if err != nil {
		return nil, err
	}
	table := new(querierv1.FunctionTable)
	if report != nil {
		if report.GetFunctions().GetFunctions() == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("query backend returned no function table"))
		}
		table = report.Functions.Functions
	}
	return table, nil
}
