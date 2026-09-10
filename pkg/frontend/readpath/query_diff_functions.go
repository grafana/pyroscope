package readpath

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func (r *Router) diffFunctions(ctx context.Context, c *connect.Request[querierv1.DiffRequest]) (*connect.Response[querierv1.DiffResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if c.Msg.Left == nil || c.Msg.Right == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("left and right selections are required"))
	}
	unsupported := connect.NewError(connect.CodeUnimplemented, errors.New("function diffs require both selections to use v2 storage"))
	// Both complete selections must use v2 for every tenant. Routing each side
	// through SelectMergeStacktraces would limit rows before the cross-profile join.
	for _, id := range tenantIDs {
		cfg := r.overrides.ReadPathOverrides(id)
		if !cfg.EnableQueryBackend {
			return nil, unsupported
		}
		split, err := cfg.EnableQueryBackendFrom.SplitTime(func() (time.Time, error) {
			return r.resolver.OldestProfileTime(ctx, id)
		})
		if err != nil {
			level.Warn(r.logger).Log("msg", "failed to resolve split time for function diff", "err", err)
			return nil, unsupported
		}
		if c.Msg.Left.Start < split.UnixMilli() || c.Msg.Right.Start < split.UnixMilli() {
			return nil, unsupported
		}
	}
	resp, err := r.newFrontend.Diff(ctx, c)
	if err != nil {
		return nil, err
	}
	if resp.Msg.Functions == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("read path does not support function diffs"))
	}
	return resp, nil
}
