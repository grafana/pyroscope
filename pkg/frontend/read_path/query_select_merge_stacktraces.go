package read_path

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryBackend) SelectMergeStacktraces(
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

func (q *QueryBackend) selectMergeStacktracesTree(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (tree []byte, err error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("max_nodes", c.Msg.GetMaxNodes()).
		SetTag("profile_type", c.Msg.ProfileTypeID)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.ValidateTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
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
	report, err := q.Query(ctx, c.Msg.Start, c.Msg.End, tenantIDs, labelSelector, &querybackendv1.Query{
		QueryType: querybackendv1.QueryType_QUERY_TREE,
		Tree:      &querybackendv1.TreeQuery{MaxNodes: maxNodes},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	return report.Tree.Tree, nil
}