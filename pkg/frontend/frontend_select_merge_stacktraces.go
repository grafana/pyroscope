package frontend

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/util/connectgrpc"
	validationutil "github.com/grafana/pyroscope/v2/pkg/util/validation"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func (f *Frontend) SelectMergeStacktraces(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	if c.Msg.Format == querierv1.ProfileFormat_PROFILE_FORMAT_DOT {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("dot format is only supported with the v2 query backend"))
	}
	// trace_id_selector is v2-only; this legacy frontend would drop it on split.
	if len(c.Msg.TraceIdSelector) > 0 {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("trace_id_selector is only supported with the v2 query backend"))
	}
	t, err := f.selectMergeStacktracesTree(ctx, c)
	if err != nil {
		return nil, err
	}
	var resp querierv1.SelectMergeStacktracesResponse
	switch c.Msg.Format {
	default:
		resp.Flamegraph = phlaremodel.NewFlameGraph(t, c.Msg.GetMaxNodes())
	case querierv1.ProfileFormat_PROFILE_FORMAT_TREE:
		resp.Tree = t.Bytes(c.Msg.GetMaxNodes(), nil)
	}
	return connect.NewResponse(&resp), nil
}

func (f *Frontend) selectMergeStacktracesTree(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*phlaremodel.FunctionNameTree, error) {
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
		return new(phlaremodel.FunctionNameTree), nil
	}
	maxNodes, err := validation.ValidateMaxNodes(f.limits, tenantIDs, c.Msg.GetMaxNodes())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent := validationutil.SmallestPositiveNonZeroIntPerTenant(tenantIDs, f.limits.MaxQueryParallelism); maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	m := phlaremodel.NewFlameGraphMerger()
	interval := validationutil.MaxDurationOrZeroPerTenant(tenantIDs, f.limits.QuerySplitDuration)
	intervals := NewTimeIntervalIterator(time.UnixMilli(int64(validated.Start)), time.UnixMilli(int64(validated.End)), interval)

	for intervals.Next() {
		r := intervals.At()
		g.Go(func() error {
			req := connectgrpc.CloneRequest(c, &querierv1.SelectMergeStacktracesRequest{
				ProfileTypeID: c.Msg.ProfileTypeID,
				LabelSelector: c.Msg.LabelSelector,
				Start:         r.Start.UnixMilli(),
				End:           r.End.UnixMilli(),
				MaxNodes:      &maxNodes,
				Format:        querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
			})
			resp, err := connectgrpc.RoundTripUnary[
				querierv1.SelectMergeStacktracesRequest,
				querierv1.SelectMergeStacktracesResponse](ctx, f, req)
			if err != nil {
				return err
			}
			if len(resp.Msg.Tree) > 0 {
				err = m.MergeTreeBytes(resp.Msg.Tree)
			} else if resp.Msg.Flamegraph != nil {
				// For backward compatibility.
				m.MergeFlameGraph(resp.Msg.Flamegraph)
			}
			return err
		})
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	return m.Tree(), nil
}
