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

func (f *Frontend) SelectMergeStacktraces(ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (
	*connect.Response[querierv1.SelectMergeStacktracesResponse], error,
) {
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
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{}), nil
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
			})
			resp, err := connectgrpc.RoundTripUnary[
				querierv1.SelectMergeStacktracesRequest,
				querierv1.SelectMergeStacktracesResponse](ctx, f, req)
			if err != nil {
				return err
			}
			m.MergeFlameGraph(resp.Msg.Flamegraph)
			return nil
		})
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	t := m.Tree()
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Flamegraph: phlaremodel.NewFlameGraph(t, c.Msg.GetMaxNodes()),
	}), nil
}
