package frontend

import (
	"context"
	"net/http"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/tenant"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
	"github.com/grafana/phlare/pkg/util/httpgrpc"
	"github.com/grafana/phlare/pkg/util/validation"
)

func (f *Frontend) SelectMergeStacktraces(ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (
	*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, httpgrpc.Errorf(http.StatusBadRequest, err.Error())
	}

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent := validation.SmallestPositiveNonZeroIntPerTenant(tenantIDs, f.limits.MaxQueryParallelism); maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	m := phlaremodel.NewFlameGraphMerger()
	interval := validation.MaxDurationOrZeroPerTenant(tenantIDs, f.limits.QuerySplitDuration)
	intervals := NewTimeIntervalIterator(time.UnixMilli(c.Msg.Start), time.UnixMilli(c.Msg.End), interval)

	for intervals.Next() {
		r := intervals.At()
		g.Go(func() error {
			req := connectgrpc.CloneRequest(c, &querierv1.SelectMergeStacktracesRequest{
				ProfileTypeID: c.Msg.ProfileTypeID,
				LabelSelector: c.Msg.LabelSelector,
				Start:         r.Start.UnixMilli(),
				End:           r.End.UnixMilli(),
				MaxNodes:      c.Msg.MaxNodes,
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

	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Flamegraph: m.FlameGraph(c.Msg.GetMaxNodes()),
	}), nil
}
