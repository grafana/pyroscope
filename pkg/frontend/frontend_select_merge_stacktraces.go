package frontend

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/util"
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
		SetTag("profile_type", c.Msg.ProfileTypeID)

	var err error
	var spanSelector []uint64
	c.Msg.LabelSelector, spanSelector, err = extractSpansSelectorFromLabelSelector(c.Msg.LabelSelector)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(spanSelector) > 0 {
		resp, err := f.SelectMergeSpanProfile(ctx, connect.NewRequest(&querierv1.SelectMergeSpanProfileRequest{
			ProfileTypeID: c.Msg.ProfileTypeID,
			LabelSelector: c.Msg.LabelSelector,
			SpanSelector:  spanSelector,
			Start:         c.Msg.Start,
			End:           c.Msg.End,
			MaxNodes:      c.Msg.MaxNodes,
		}))
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Flamegraph: resp.Msg.Flamegraph}), nil
	}

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

	t := m.Tree()
	t.FormatNodeNames(phlaremodel.DropGoTypeParameters)
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Flamegraph: phlaremodel.NewFlameGraph(t, c.Msg.GetMaxNodes()),
	}), nil
}

func extractSpansSelectorFromLabelSelector(labelSelector string) (ls string, ss []uint64, err error) {
	matchers, err := parser.ParseMetricSelector(labelSelector)
	if err != nil {
		return "", nil, err
	}
	var spanMatcher *labels.Matcher
	for i, m := range matchers {
		if m.Name != pprof.SpanIDLabelName && m.Name != pprof.ProfileIDLabelName {
			continue
		}
		if m.Type != labels.MatchEqual {
			return "", nil, fmt.Errorf("span matcher only supports '=' operator")
		}
		matchers = append(matchers[:i], matchers[i+1:]...)
		spanMatcher = m
	}
	if spanMatcher == nil {
		return labelSelector, nil, nil
	}
	spanSelector, err := spansFromMatcherValue(spanMatcher.Value)
	if err != nil {
		return "", nil, fmt.Errorf("invalid span selector: %w", err)
	}
	var b strings.Builder
	b.WriteRune('{')
	for _, m := range matchers {
		b.WriteString(m.String())
		b.WriteRune(',')
	}
	b.WriteRune('}')
	return b.String(), spanSelector, nil
}

func spansFromMatcherValue(s string) ([]uint64, error) {
	tmp := make([]byte, 8)
	spans := make([]uint64, 0, len(s)/17)
	for _, x := range bytes.Split(util.YoloBuf(s), []byte(",")) {
		if len(x) != 16 {
			return nil, fmt.Errorf("invalid span id lenth: %d", len(x))
		}
		if _, err := hex.Decode(tmp, x); err != nil {
			return nil, err
		}
		spans = append(spans, binary.LittleEndian.Uint64(tmp))
	}
	return spans, nil
}
