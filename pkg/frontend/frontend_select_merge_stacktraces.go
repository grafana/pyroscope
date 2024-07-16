package frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) SelectMergeStacktraces(ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	t, err := f.selectMergeStacktracesTree(ctx, c)
	if err != nil {
		return nil, err
	}
	var resp querierv1.SelectMergeStacktracesResponse
	switch c.Msg.Format {
	default:
		tree := phlaremodel.MustUnmarshalTree(t)
		resp.Flamegraph = phlaremodel.NewFlameGraph(tree, c.Msg.GetMaxNodes())
	case querierv1.ProfileFormat_PROFILE_FORMAT_TREE:
		resp.Tree = t
	}
	return connect.NewResponse(&resp), nil
}

func (f *Frontend) selectMergeStacktracesTree(ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) ([]byte, error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("max_nodes", c.Msg.GetMaxNodes()).
		SetTag("profile_type", c.Msg.ProfileTypeID)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSelectMergeStacktracesProcedure)
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, model.Interval{Start: model.Time(c.Msg.Start), End: model.Time(c.Msg.End)}, model.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if validated.IsEmpty {
		return nil, nil
	}

	labelSelector, err := buildLabelSelectorAndProfileType(c.Msg.LabelSelector, c.Msg.ProfileTypeID)
	if err != nil {
		return nil, err
	}
	report, err := f.invoke(ctx, c.Msg.Start, c.Msg.End, tenantIDs, labelSelector, &querybackendv1.Query{
		QueryType: querybackendv1.QueryType_QUERY_TREE,
		Tree: &querybackendv1.TreeQuery{
			MaxNodes: c.Msg.GetMaxNodes(),
		},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	return report.Tree.Tree, nil
}
