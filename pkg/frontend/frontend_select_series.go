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

func (f *Frontend) SelectSeries(ctx context.Context,
	c *connect.Request[querierv1.SelectSeriesRequest]) (
	*connect.Response[querierv1.SelectSeriesResponse], error,
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
		return connect.NewResponse(&querierv1.SelectSeriesResponse{}), nil
	}
	c.Msg.Start = int64(validated.Start)
	c.Msg.End = int64(validated.End)

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent := validationutil.SmallestPositiveNonZeroIntPerTenant(tenantIDs, f.limits.MaxQueryParallelism); maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	m := phlaremodel.NewSeriesMerger(false)
	interval := validationutil.MaxDurationOrZeroPerTenant(tenantIDs, f.limits.QuerySplitDuration)
	intervals := NewTimeIntervalIterator(time.UnixMilli(c.Msg.Start), time.UnixMilli(c.Msg.End), interval,
		WithAlignment(time.Second*time.Duration(c.Msg.Step)))

	for intervals.Next() {
		r := intervals.At()
		g.Go(func() error {
			req := connectgrpc.CloneRequest(c, &querierv1.SelectSeriesRequest{
				ProfileTypeID: c.Msg.ProfileTypeID,
				LabelSelector: c.Msg.LabelSelector,
				Start:         r.Start.UnixMilli(),
				End:           r.End.UnixMilli(),
				GroupBy:       c.Msg.GroupBy,
				Step:          c.Msg.Step,
			})
			resp, err := connectgrpc.RoundTripUnary[
				querierv1.SelectSeriesRequest,
				querierv1.SelectSeriesResponse](ctx, f, req)
			if err != nil {
				return err
			}
			m.MergeSeries(resp.Msg.Series)
			return nil
		})
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	return connect.NewResponse(&querierv1.SelectSeriesResponse{Series: m.Series()}), nil
}
