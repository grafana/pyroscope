package frontend

import (
	"context"
	"net/http"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	validationutil "github.com/grafana/pyroscope/pkg/util/validation"
	"github.com/grafana/pyroscope/pkg/validation"
)

type resultOrder[A any] struct {
	idx    int
	result A
}

func mergeResultInOrder[A any](ch <-chan resultOrder[A], merge func(A)) error {
	var results []resultOrder[A]

	nextIdx := 0
	for {
		// check if first element matches
		if len(results) > 0 && results[0].idx == nextIdx {
			merge(results[0].result)
			results = results[1:]
			nextIdx++
			continue
		}

		// wait for incoming result or channel close
		r, ok := <-ch
		if !ok {
			if len(results) > 0 {
				return connect.NewError(http.StatusInternalServerError, errors.New("channel closed before all results were received"))
			}
			return nil
		}

		// result comes in at right order
		if r.idx == nextIdx {
			merge(r.result)
			nextIdx++
			continue
		}

		// add element to slice at the right position
		i := 0
		for ; i < len(results); i++ {
			if results[i].idx > r.idx {
				break
			}
		}
		results = append(results[:i], append([]resultOrder[A]{r}, results[i:]...)...)

	}
}

func (f *Frontend) SelectMergeStacktraces(ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (
	*connect.Response[querierv1.SelectMergeStacktracesResponse], error,
) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("selector", c.Msg.LabelSelector).
		SetTag("profile_type", c.Msg.ProfileTypeID)

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSelectMergeStacktracesProcedure)
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

	g, ctx := errgroup.WithContext(ctx)
	if maxConcurrent := validationutil.SmallestPositiveNonZeroIntPerTenant(tenantIDs, f.limits.MaxQueryParallelism); maxConcurrent > 0 {
		g.SetLimit(maxConcurrent)
	}

	m := phlaremodel.NewFlameGraphMerger()
	interval := validationutil.MaxDurationOrZeroPerTenant(tenantIDs, f.limits.QuerySplitDuration)
	intervals := NewTimeIntervalIterator(time.UnixMilli(int64(validated.Start)), time.UnixMilli(int64(validated.End)), interval)

	// prepare the channel for the results
	mergeCh := make(chan resultOrder[*querierv1.FlameGraph])

	mergeErrCh := make(chan error)
	go func() {
		mergeErrCh <- mergeResultInOrder(mergeCh, m.MergeFlameGraph)
	}()

	idx := 0
	for intervals.Next() {
		r := intervals.At()
		var midx = idx
		idx++
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
			mergeCh <- resultOrder[*querierv1.FlameGraph]{idx: midx, result: resp.Msg.Flamegraph}
			return nil
		})
	}

	if err = g.Wait(); err != nil {
		close(mergeCh)
		return nil, err
	}

	// signal to merge loop that all results have arrived
	close(mergeCh)

	if err = <-mergeErrCh; err != nil {
		return nil, err
	}

	t := m.Tree()
	t.FormatNodeNames(phlaremodel.DropGoTypeParameters)
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Flamegraph: phlaremodel.NewFlameGraph(t, c.Msg.GetMaxNodes()),
	}), nil
}
