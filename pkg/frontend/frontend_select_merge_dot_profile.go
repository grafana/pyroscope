package frontend

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/pyroscope/pkg/frontend/dot/graph"
	"github.com/grafana/pyroscope/pkg/frontend/dot/report"
	"github.com/opentracing/opentracing-go"
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

func (f *Frontend) SelectMergeDotProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[querierv1.SelectMergeDotProfileResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("max_nodes", c.Msg.GetMaxNodes()).
		SetTag("profile_type", c.Msg.ProfileTypeID)

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
		return connect.NewResponse(&querierv1.SelectMergeDotProfileResponse{}), nil
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

	var lock sync.Mutex
	var m pprof.ProfileMerge
	for intervals.Next() {
		r := intervals.At()
		g.Go(func() error {
			req := connectgrpc.CloneRequest(c, &querierv1.SelectMergeProfileRequest{
				ProfileTypeID: c.Msg.ProfileTypeID,
				LabelSelector: c.Msg.LabelSelector,
				Start:         r.Start.UnixMilli(),
				End:           r.End.UnixMilli(),
				MaxNodes:      c.Msg.MaxNodes,
			})
			resp, err := connectgrpc.RoundTripUnary[
				querierv1.SelectMergeProfileRequest,
				profilev1.Profile](ctx, f, req)
			if err != nil {
				return err
			}
			lock.Lock()
			defer lock.Unlock()
			return m.Merge(resp.Msg)
		})
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	p := m.Profile()

	data, err := p.MarshalVT()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	pr, err := profile.ParseData(data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	nodes := 100
	if c.Msg.MaxNodes != nil {
		nodes = int(*c.Msg.MaxNodes)
	}
	rpt := report.NewDefault(pr, report.Options{
		NodeCount: nodes,
	})
	gr, cfg := report.GetDOT(rpt)

	// Create a byte slice to hold the written data
	var buf bytes.Buffer

	// Create a writer backed by the byte slice
	writer := io.Writer(&buf)
	graph.ComposeDot(writer, gr, &graph.DotAttributes{}, cfg)

	return connect.NewResponse(&querierv1.SelectMergeDotProfileResponse{
		Profile: buf.String(),
	}), nil
}
