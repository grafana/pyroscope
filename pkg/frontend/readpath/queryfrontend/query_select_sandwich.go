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

func (q *QueryFrontend) selectMergeStacktracesSandwich(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	if c.Msg.GetSandwichFunction() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("sandwich_function is required for the sandwich format"))
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
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Sandwich: new(querierv1.SandwichReport)}), nil
	}
	maxNodes, err := validation.ValidateMaxNodes(q.limits, tenantIDs, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := q.querySandwich(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	// Per half, so a wide callee side cannot starve the callers.
	var merger model.SandwichMerger
	merger.Merge(report)
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Sandwich: merger.Sandwich(maxNodes),
	}), nil
}

// querySandwich returns both halves whole for a validated time range. Callers
// apply the node budget only after all aggregation, because a half truncated
// earlier cannot be corrected by a later merge.
func (q *QueryFrontend) querySandwich(ctx context.Context, req *querierv1.SelectMergeStacktracesRequest) (*querierv1.SandwichReport, error) {
	if _, err := model.ParseProfileTypeSelector(req.ProfileTypeID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	selector, err := buildLabelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	// TODO: Native symbolization is deliberately omitted, as for function tables.
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime: req.Start, EndTime: req.End, LabelSelector: selector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SANDWICH,
			Sandwich: &queryv1.SandwichQuery{
				Function:           req.GetSandwichFunction(),
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
	result := new(querierv1.SandwichReport)
	if report != nil {
		if report.GetSandwich().GetSandwich() == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("query backend returned no sandwich report"))
		}
		result = report.Sandwich.Sandwich
	}
	return result, nil
}
