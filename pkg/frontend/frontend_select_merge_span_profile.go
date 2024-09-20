package frontend

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	validationutil "github.com/grafana/pyroscope/pkg/util/validation"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) SelectMergeSpanProfile(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeSpanProfileRequest],
) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("max_nodes", c.Msg.MaxNodes).
		SetTag("profile_type", c.Msg.ProfileTypeID)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSelectMergeSpanProfileProcedure)
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, model.Interval{Start: model.Time(c.Msg.Start), End: model.Time(c.Msg.End)}, model.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if validated.IsEmpty {
		return connect.NewResponse(&querierv1.SelectMergeSpanProfileResponse{Flamegraph: &querierv1.FlameGraph{}}), nil
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
			req := connectgrpc.CloneRequest(c, &querierv1.SelectMergeSpanProfileRequest{
				ProfileTypeID: c.Msg.ProfileTypeID,
				LabelSelector: c.Msg.LabelSelector,
				Start:         r.Start.UnixMilli(),
				End:           r.End.UnixMilli(),
				MaxNodes:      &maxNodes,
				SpanSelector:  c.Msg.SpanSelector,
				Format:        querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
			})
			resp, err := connectgrpc.RoundTripUnary[
				querierv1.SelectMergeSpanProfileRequest,
				querierv1.SelectMergeSpanProfileResponse](ctx, f, req)
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

	t := m.Tree()
	var resp querierv1.SelectMergeSpanProfileResponse
	switch c.Msg.Format {
	default:
		resp.Flamegraph = phlaremodel.NewFlameGraph(t, c.Msg.GetMaxNodes())
	case querierv1.ProfileFormat_PROFILE_FORMAT_TREE:
		resp.Tree = t.Bytes(c.Msg.GetMaxNodes())
	}
	return connect.NewResponse(&resp), nil
}
