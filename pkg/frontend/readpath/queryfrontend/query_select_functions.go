package queryfrontend

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
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
	if _, err := model.ParseProfileTypeSelector(c.Msg.ProfileTypeID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	selector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime: c.Msg.Start, EndTime: c.Msg.End, LabelSelector: selector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_FUNCTIONS,
			Functions: &queryv1.FunctionsQuery{
				SpanSelector:       c.Msg.SpanSelector,
				StackTraceSelector: c.Msg.StackTraceSelector,
				ProfileIdSelector:  c.Msg.ProfileIdSelector,
				TraceIdSelector:    c.Msg.TraceIdSelector,
			},
		}},
	}, func(ctx context.Context, upstream QueryBackend, blocks []*metastorev1.BlockMeta) QueryBackend {
		if q.useSymbolRefTrees(tenantIDs) || q.shouldSymbolize(ctx, tenantIDs, blocks) {
			return &backendFunctionsSymbolizer{upstream: upstream, frontend: q, tenants: tenantIDs}
		}
		return upstream
	})
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
	model.LimitFunctionTable(table, maxNodes)
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Functions: table}), nil
}

// Deferred symbolization can rename frames and introduce inline frames. Keep
// those stacks intact until resolution, so recursion is counted by the final
// function name. Already symbolized queries use compact function reports.
type backendFunctionsSymbolizer struct {
	upstream QueryBackend
	frontend *QueryFrontend
	tenants  []string
}

func (b *backendFunctionsSymbolizer) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	modified := req.CloneVT()
	for _, query := range modified.Query {
		if query.QueryType != queryv1.QueryType_QUERY_FUNCTIONS {
			continue
		}
		f := query.Functions
		query.QueryType = queryv1.QueryType_QUERY_TREE
		query.Tree = &queryv1.TreeQuery{
			SpanSelector: f.SpanSelector, StackTraceSelector: f.StackTraceSelector,
			ProfileIdSelector: f.ProfileIdSelector, TraceIdSelector: f.TraceIdSelector,
		}
		b.frontend.symbolRefTreeQuery(query.Tree, b.tenants)
		query.Functions = nil
	}
	resp, err := b.upstream.Invoke(ctx, modified)
	if err != nil {
		return nil, err
	}
	for _, report := range resp.Reports {
		if report.ReportType != queryv1.ReportType_REPORT_TREE {
			continue
		}
		if err := b.frontend.resolveSymbolRefs(ctx, report, 0); err != nil {
			return nil, err
		}
		tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](report.Tree.Tree)
		if err != nil {
			return nil, fmt.Errorf("unmarshal functions tree: %w", err)
		}
		table, err := model.FunctionTableFromTree(ctx, tree)
		if err != nil {
			return nil, err
		}
		for _, query := range req.Query {
			if query.QueryType == queryv1.QueryType_QUERY_FUNCTIONS {
				report.Functions = &queryv1.FunctionsReport{Query: query.Functions.CloneVT(), Functions: table}
				break
			}
		}
		report.ReportType = queryv1.ReportType_REPORT_FUNCTIONS
		report.Tree = nil
	}
	return resp, nil
}
