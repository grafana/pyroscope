package queryfrontend

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	"github.com/grafana/pyroscope/v2/pkg/validation"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func (q *QueryFrontend) selectMergeStacktracesProjection(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	req := c.Msg.CloneVT()
	if err := validateProjectionRequest(req); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	selector, err := buildLabelSelectorWithProfileType(req.LabelSelector, req.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &req.Start, &req.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(projectionResponse(req, nil)), nil
	}
	query := &queryv1.Query{
		QueryType: queryv1.QueryType_QUERY_FUNCTIONS,
		Functions: &queryv1.FunctionsQuery{
			StackTraceSelector: req.StackTraceSelector,
			ProfileIdSelector:  req.ProfileIdSelector,
			TraceIdSelector:    req.TraceIdSelector,
			SpanSelector:       req.SpanSelector,
		},
	}
	// Stored names only: projection cannot use post-tree native symbolization.
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: selector,
		Query:         []*queryv1.Query{query},
	}, nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(projectionResponse(req, report)), nil
}

func validateProjectionRequest(req *querierv1.SelectMergeStacktracesRequest) error {
	if req == nil {
		return errors.New("projection request is required")
	}
	if req.MaxNodes != nil {
		return errors.New("function projections use format options, not max_nodes")
	}
	selector := req.GetStackTraceSelector()
	if selector.GetGoPgo() != nil {
		return errors.New("function projections do not support go_pgo")
	}
	for _, loc := range selector.GetCallSite() {
		if loc.GetName() == "" {
			return errors.New("call_site requires nonempty function names")
		}
	}
	if req.Format != querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS {
		return errors.New("invalid projection format")
	}
	if req.GetFormatOptions().GetFunctions().GetLimit() < 0 {
		return errors.New("function limit must not be negative")
	}
	return nil
}

func projectionResponse(req *querierv1.SelectMergeStacktracesRequest, report *queryv1.Report) *querierv1.SelectMergeStacktracesResponse {
	limit := req.GetFormatOptions().GetFunctions().GetLimit()
	table := report.GetFunctions().GetTable()
	if table == nil {
		table = new(typesv1.FunctionTable)
	} else if limit > 0 && int64(len(table.Functions)) > limit {
		table = &typesv1.FunctionTable{
			Functions:      table.Functions[:limit],
			Total:          table.Total,
			TotalFunctions: table.TotalFunctions,
		}
	}
	return &querierv1.SelectMergeStacktracesResponse{Functions: table}
}
