package queryfrontend

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend/dot"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func (q *QueryFrontend) SelectMergeStacktraces(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	if len(c.Msg.SpanSelector) > 0 && len(c.Msg.TraceIdSelector) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("span_selector and trace_id_selector cannot be combined"))
	}

	switch c.Msg.Format {
	case querierv1.ProfileFormat_PROFILE_FORMAT_DOT:
		return q.selectMergeStacktracesDot(ctx, c)
	case querierv1.ProfileFormat_PROFILE_FORMAT_PPROF:
		p, err := q.selectMergeStacktracesPprof(ctx, c.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
			Pprof: &querierv1.PprofProfile{Profile: p},
		}), nil
	}

	b, err := q.selectMergeStacktracesTree(ctx, c)
	if err != nil {
		return nil, err
	}
	var resp querierv1.SelectMergeStacktracesResponse
	switch c.Msg.Format {
	case querierv1.ProfileFormat_PROFILE_FORMAT_TREE:
		resp.Tree = b
	default:
		t, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](b)
		if err != nil {
			return nil, err
		}
		resp.Flamegraph = phlaremodel.NewFlameGraph(t, c.Msg.GetMaxNodes())
	}
	return connect.NewResponse(&resp), nil
}

func (q *QueryFrontend) selectMergeStacktracesDot(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	// Use separate max nodes for source pprof fetch vs DOT rendering
	const defaultSourceMaxNodes = int64(512)
	const defaultDotMaxNodes = int64(100)
	dotMaxNodes := defaultDotMaxNodes
	sourceMaxNodes := defaultSourceMaxNodes
	if c.Msg.MaxNodes != nil {
		if v := c.Msg.GetMaxNodes(); v > 0 {
			dotMaxNodes = v
		}
		if dotMaxNodes > sourceMaxNodes {
			sourceMaxNodes = dotMaxNodes
		}
	}

	profile, err := q.selectMergeStacktracesPprof(ctx, &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID:      c.Msg.ProfileTypeID,
		LabelSelector:      c.Msg.LabelSelector,
		Start:              c.Msg.Start,
		End:                c.Msg.End,
		MaxNodes:           &sourceMaxNodes,
		StackTraceSelector: c.Msg.StackTraceSelector,
		ProfileIdSelector:  c.Msg.ProfileIdSelector,
		TraceIdSelector:    c.Msg.TraceIdSelector,
		SpanSelector:       c.Msg.SpanSelector,
	})
	if err != nil {
		return nil, err
	}
	if profile == nil || len(profile.Sample) == 0 {
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{}), nil
	}

	d, err := dot.FromProfile(profile, int(dotMaxNodes))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Dot: d}), nil
}

func (q *QueryFrontend) selectMergeStacktracesTree(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (tree []byte, err error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return nil, nil
	}

	maxNodes, err := validation.ValidateMaxNodes(q.limits, tenantIDs, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	_, err = phlaremodel.ParseProfileTypeSelector(c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	labelSelector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	useSymbolRefs := q.useSymbolRefTrees(tenantIDs)
	treeQuery := &queryv1.TreeQuery{
		MaxNodes:           maxNodes,
		StackTraceSelector: c.Msg.StackTraceSelector,
		ProfileIdSelector:  c.Msg.ProfileIdSelector,
		TraceIdSelector:    c.Msg.TraceIdSelector,
		SpanSelector:       c.Msg.SpanSelector,
	}
	if useSymbolRefs {
		q.symbolRefTreeQuery(treeQuery, tenantIDs)
	}
	report, err := q.querySingle(ctx,
		&queryv1.QueryRequest{
			StartTime:     c.Msg.Start,
			EndTime:       c.Msg.End,
			LabelSelector: labelSelector,
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_TREE,
				Tree:      treeQuery,
			}},
		},
		func(ctx context.Context, upstream QueryBackend, blocks []*metastorev1.BlockMeta) QueryBackend {
			if useSymbolRefs {
				return upstream
			}
			shouldSymbolize := q.shouldSymbolize(ctx, tenantIDs, blocks)
			if !shouldSymbolize {
				return upstream
			}
			return &backendTreeSymbolizer{
				upstream:   upstream,
				symbolizer: q.symbolizer,
			}
		},
	)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	if err := q.resolveSymbolRefs(ctx, tenantIDs, report, maxNodes); err != nil {
		return nil, err
	}
	return report.Tree.Tree, nil
}
