package frontend

import (
	"context"
	"net/http"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
	validationutil "github.com/grafana/phlare/pkg/util/validation"
	"github.com/grafana/phlare/pkg/validation"
)

func (f *Frontend) SelectMergeStacktraces(ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (
	*connect.Response[querierv1.SelectMergeStacktracesResponse], error,
) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(http.StatusBadRequest, err)
	}

	validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, model.Interval{Start: model.Time(c.Msg.Start), End: model.Time(c.Msg.End)}, model.Now())
	if err != nil {
		return nil, connect.NewError(http.StatusBadRequest, err)
	}
	if validated.IsEmpty {
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{}), nil
	}
	c.Msg.Start = int64(validated.Start)
	c.Msg.End = int64(validated.End)

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent := validationutil.SmallestPositiveNonZeroIntPerTenant(tenantIDs, f.limits.MaxQueryParallelism); maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	m := phlaremodel.NewFlameGraphMerger()
	interval := validationutil.MaxDurationOrZeroPerTenant(tenantIDs, f.limits.QuerySplitDuration)
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
