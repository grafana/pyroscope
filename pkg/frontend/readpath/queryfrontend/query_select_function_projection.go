package queryfrontend

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/validation"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// Keep in sync with the public range documented on types.v1.FunctionTreeOptions.max_depth.
const maxFunctionTreeDepth = 128

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
	var query queryv1.Query
	switch req.Format {
	case querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS:
		query.QueryType = queryv1.QueryType_QUERY_FUNCTIONS
		query.Functions = &queryv1.FunctionsQuery{
			StackTraceSelector: req.StackTraceSelector,
			ProfileIdSelector:  req.ProfileIdSelector,
			TraceIdSelector:    req.TraceIdSelector,
			SpanSelector:       req.SpanSelector,
		}
	case querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE:
		query.QueryType = queryv1.QueryType_QUERY_FUNCTION_TREE
		query.FunctionTree = &queryv1.FunctionTreeQuery{
			Options:            req.GetFormatOptions().GetFunctionTree(),
			StackTraceSelector: req.StackTraceSelector,
			ProfileIdSelector:  req.ProfileIdSelector,
			TraceIdSelector:    req.TraceIdSelector,
			SpanSelector:       req.SpanSelector,
		}
	}
	// Stored names only: projection cannot use post-tree native symbolization.
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     req.Start,
		EndTime:       req.End,
		LabelSelector: selector,
		Query:         []*queryv1.Query{&query},
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
	options := req.GetFormatOptions()
	switch req.Format {
	case querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS:
		limit := options.GetFunctions().GetLimit()
		if limit < 0 {
			return errors.New("function limit must not be negative")
		}
	case querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE:
		if options.GetFunctionTree() == nil {
			return errors.New("FUNCTION_TREE requires format_options.function_tree")
		}
		o := options.GetFunctionTree()
		switch o.GetDirection() {
		case typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
			typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS,
			typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH:
		default:
			return errors.New("function tree requires a valid direction")
		}
		if depth := o.GetMaxDepth(); depth < 0 || depth > maxFunctionTreeDepth {
			return fmt.Errorf("max_depth must be between 0 and %d", maxFunctionTreeDepth)
		}
		switch o.GetSelection() {
		case typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH:
			if o.GetDirection() != typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES {
				return errors.New("ROOT_PATH supports CALLEES only")
			}
		case typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN:
			count := len(selector.GetCallSite())
			if count == 0 || (o.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH && count != 1) {
				return errors.New("FUNCTION_CHAIN requires a function; BOTH requires exactly one function")
			}
		default:
			return errors.New("function tree requires a valid selection")
		}
	default:
		return errors.New("invalid projection format")
	}
	return nil
}

func projectionResponse(req *querierv1.SelectMergeStacktracesRequest, report *queryv1.Report) *querierv1.SelectMergeStacktracesResponse {
	if req.Format == querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE {
		tree := report.GetFunctionTree().GetTree()
		if tree == nil {
			tree = model.EmptyFunctionTree(req.GetFormatOptions().GetFunctionTree())
		}
		return &querierv1.SelectMergeStacktracesResponse{FunctionTree: tree}
	}
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
