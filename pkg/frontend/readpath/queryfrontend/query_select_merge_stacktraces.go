package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

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
	if c.Msg.Format == querierv1.ProfileFormat_PROFILE_FORMAT_DOT {
		return q.selectMergeStacktracesDot(ctx, c)
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

	pprofResp, err := q.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID:      c.Msg.ProfileTypeID,
		LabelSelector:      c.Msg.LabelSelector,
		Start:              c.Msg.Start,
		End:                c.Msg.End,
		MaxNodes:           &sourceMaxNodes,
		StackTraceSelector: c.Msg.StackTraceSelector,
		ProfileIdSelector:  c.Msg.ProfileIdSelector,
	}))
	if err != nil {
		return nil, err
	}
	if pprofResp.Msg == nil || len(pprofResp.Msg.Sample) == 0 {
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{}), nil
	}

	d, err := dot.FromProfile(pprofResp.Msg, int(dotMaxNodes))
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
	report, err := q.querySingle(ctx,
		&queryv1.QueryRequest{
			StartTime:     c.Msg.Start,
			EndTime:       c.Msg.End,
			LabelSelector: labelSelector,
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_TREE,
				Tree: &queryv1.TreeQuery{
					MaxNodes:           maxNodes,
					StackTraceSelector: c.Msg.StackTraceSelector,
					ProfileIdSelector:  c.Msg.ProfileIdSelector,
				},
			}},
		},
	)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	return report.Tree.Tree, nil
}
