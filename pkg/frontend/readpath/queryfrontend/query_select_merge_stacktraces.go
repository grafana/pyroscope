package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) SelectMergeStacktraces(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	b, err := q.selectMergeStacktracesTree(ctx, c)
	if err != nil {
		return nil, err
	}
	var resp querierv1.SelectMergeStacktracesResponse
	switch c.Msg.Format {
	case querierv1.ProfileFormat_PROFILE_FORMAT_TREE:
		resp.Tree = b
	default:
		t, err := phlaremodel.UnmarshalTree(b)
		if err != nil {
			return nil, err
		}
		resp.Flamegraph = phlaremodel.NewFlameGraph(t, c.Msg.GetMaxNodes())
	}
	return connect.NewResponse(&resp), nil
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
	labelSelector, err := buildLabelSelectorWithProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, err
	}
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     c.Msg.Start,
		EndTime:       c.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree: &queryv1.TreeQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: c.Msg.StackTraceSelector,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	return report.Tree.Tree, nil
}
