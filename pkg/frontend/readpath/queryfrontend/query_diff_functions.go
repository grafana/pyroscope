package queryfrontend

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func (q *QueryFrontend) diffFunctions(ctx context.Context, c *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if c.Msg.Left == nil || c.Msg.Right == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("left and right selections are required"))
	}
	if _, err := model.ParseProfileTypeSelector(c.Msg.Left.ProfileTypeID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if c.Msg.Left.ProfileTypeID != c.Msg.Right.ProfileTypeID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile types must match"))
	}
	if c.Msg.Left.Async != nil || c.Msg.Right.Async != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("diff does not support per-side async requests"))
	}
	maxNodes, err := validation.ValidateMaxNodes(q.limits, tenantIDs, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Clone both sides independently: callers can reuse the same selection, and
	// sanitizing time ranges must not mutate the request or race with the other side.
	selections := []*querierv1.SelectMergeStacktracesRequest{c.Msg.Left.CloneVT(), c.Msg.Right.CloneVT()}
	tables := make([]*querierv1.FunctionTable, len(selections))
	g, queryCtx := errgroup.WithContext(ctx)
	for i, selection := range selections {
		g.Go(func() error {
			empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &selection.Start, &selection.End)
			if err != nil {
				return connect.NewError(connect.CodeInvalidArgument, err)
			}
			if empty {
				return nil
			}
			tables[i], err = q.queryFunctionTable(queryCtx, selection)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	diff, err := model.NewFunctionTableDiff(ctx, tables[0], tables[1], maxNodes)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&querierv1.DiffResponse{Functions: diff}), nil
}
