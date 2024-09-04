package query_frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) Diff(
	ctx context.Context,
	c *connect.Request[querierv1.DiffRequest],
) (*connect.Response[querierv1.DiffResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	maxNodes := c.Msg.Left.GetMaxNodes()
	if n := c.Msg.Right.GetMaxNodes(); n > maxNodes {
		maxNodes = n
	}
	maxNodes, err = validation.ValidateMaxNodes(q.limits, tenantIDs, maxNodes)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	c.Msg.Left.MaxNodes = &maxNodes
	c.Msg.Right.MaxNodes = &maxNodes

	var left, right []byte
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var leftErr error
		left, leftErr = q.selectMergeStacktracesTree(ctx, connect.NewRequest(c.Msg.Left))
		return leftErr
	})
	g.Go(func() error {
		var rightErr error
		right, rightErr = q.selectMergeStacktracesTree(ctx, connect.NewRequest(c.Msg.Right))
		return rightErr
	})
	if err = g.Wait(); err != nil {
		return nil, err
	}

	diff, err := phlaremodel.NewFlamegraphDiffFromBytes(left, right, maxNodes)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&querierv1.DiffResponse{Flamegraph: diff}), nil
}
