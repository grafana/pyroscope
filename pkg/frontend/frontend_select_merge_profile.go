package frontend

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	validationutil "github.com/grafana/pyroscope/pkg/util/validation"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) SelectMergeProfile(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeProfileRequest],
) (*connect.Response[profilev1.Profile], error) {
	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSelectMergeProfileProcedure)
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, model.Interval{Start: model.Time(c.Msg.Start), End: model.Time(c.Msg.End)}, model.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if validated.IsEmpty {
		return connect.NewResponse(&profilev1.Profile{}), nil
	}
	c.Msg.Start = int64(validated.Start)
	c.Msg.End = int64(validated.End)

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent := validationutil.SmallestPositiveNonZeroIntPerTenant(tenantIDs, f.limits.MaxQueryParallelism); maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	interval := validationutil.MaxDurationOrZeroPerTenant(tenantIDs, f.limits.QuerySplitDuration)
	intervals := NewTimeIntervalIterator(time.UnixMilli(int64(validated.Start)), time.UnixMilli(int64(validated.End)), interval)

	// NOTE: Max nodes limit is not set by default:
	//   the method is used for pprof export and
	//   truncation is not applicable for that.
	maxNodesEnabled := false
	for _, tenantID := range tenantIDs {
		if f.limits.MaxFlameGraphNodesOnSelectMergeProfile(tenantID) {
			maxNodesEnabled = true
		}
	}

	maxNodes := c.Msg.MaxNodes
	if maxNodesEnabled {
		maxNodesV, err := validation.ValidateMaxNodes(f.limits, tenantIDs, c.Msg.GetMaxNodes())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		maxNodes = &maxNodesV
	}

	var m pprof.ProfileMerge
	for intervals.Next() {
		r := intervals.At()
		g.Go(func() error {
			req := connectgrpc.CloneRequest(c, &querierv1.SelectMergeProfileRequest{
				ProfileTypeID:      c.Msg.ProfileTypeID,
				LabelSelector:      c.Msg.LabelSelector,
				Start:              r.Start.UnixMilli(),
				End:                r.End.UnixMilli(),
				MaxNodes:           maxNodes,
				StackTraceSelector: c.Msg.StackTraceSelector,
			})
			resp, err := connectgrpc.RoundTripUnary[
				querierv1.SelectMergeProfileRequest,
				profilev1.Profile](ctx, f, req)
			if err != nil {
				return err
			}
			return m.Merge(resp.Msg, f.limits.QuerySanitizeOnMerge(tenantIDs[0]))
		})
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	return connect.NewResponse(m.Profile()), nil
}
