package distributor

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/v2/pkg/distributor/model"
	"github.com/grafana/pyroscope/v2/pkg/distributor/writepath"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	pprof2 "github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// probeSegmentWriter is a SegmentWriterClient that observes the concurrency of
// PushBatch's per-series fan-out. Tests route every series through the segment
// writer (see newProbeDistributor), whose fast path calls Push synchronously
// inside the errgroup-bounded goroutine — so the number of concurrent Push
// calls equals the number of concurrently executing pushSeries.
//
// Push sleeps briefly while holding a slot so overlapping calls are observed;
// maxInFlight records the peak fan-out using atomics only (no channels, no
// timing windows).
type probeSegmentWriter struct {
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	pushed      atomic.Int64

	// failServices / panicServices select, by service_name, the calls that
	// return an error or panic respectively. They are populated before PushBatch
	// runs and only read afterwards, so concurrent reads are race-free.
	failServices  map[string]bool
	panicServices map[string]bool
}

func (p *probeSegmentWriter) CheckReady(context.Context) error { return nil }

func (p *probeSegmentWriter) Push(_ context.Context, req *segmentwriterv1.PushRequest) (*segmentwriterv1.PushResponse, error) {
	svc := phlaremodel.Labels(req.Labels).Get(phlaremodel.LabelNameServiceName)

	n := p.inFlight.Add(1)
	for {
		m := p.maxInFlight.Load()
		if n <= m || p.maxInFlight.CompareAndSwap(m, n) {
			break
		}
	}
	// Hold the slot briefly so concurrent calls overlap and maxInFlight reflects
	// the real peak fan-out.
	time.Sleep(2 * time.Millisecond)
	p.inFlight.Add(-1)

	if p.panicServices[svc] {
		panic(fmt.Sprintf("probe panic for %s", svc))
	}
	if p.failServices[svc] {
		return nil, fmt.Errorf("probe failure for %s", svc)
	}
	p.pushed.Add(1)
	return &segmentwriterv1.PushResponse{}, nil
}

// newProbeDistributor builds a distributor whose write path routes every series
// to the segment-writer probe (synchronous fast path), with the fan-out bounded
// to maxConcurrency.
func newProbeDistributor(t *testing.T, sw SegmentWriterClient, maxConcurrency int) *Distributor {
	t.Helper()
	overrides := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.WritePathOverrides.WritePath = writepath.SegmentWriterPath
		l.PushMaxConcurrency = maxConcurrency
		tenantLimits["user-1"] = l
	})
	d, err := New(
		Config{DistributorRing: ringConfig},
		testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
		&poolFactory{f: func(addr string) (client.PoolClient, error) { return newFakeIngester(t, false), nil }},
		overrides, nil, log.NewNopLogger(), sw,
	)
	require.NoError(t, err)
	return d
}

// probeProfileSeries builds n distinct-label series, each carrying a minimal
// valid profile that splits into exactly one segment-writer request (so PushBatch
// takes the synchronous fast path). Series i is tagged service_name=svc-i.
func probeProfileSeries(n int) []*distributormodel.ProfileSeries {
	series := make([]*distributormodel.ProfileSeries, n)
	for i := range series {
		series[i] = &distributormodel.ProfileSeries{
			Labels: []*typesv1.LabelPair{
				{Name: ProfileName, Value: "process_cpu"},
				{Name: phlaremodel.LabelNameServiceName, Value: fmt.Sprintf("svc-%d", i)},
			},
			Profile: newProbeProfile(),
		}
	}
	return series
}

// newProbeProfile returns a minimal, valid profile: one sample type, one sample
// with a non-zero value (survives normalization) and no sample labels (so the
// segment-writer split yields exactly one series/request).
func newProbeProfile() *pprof2.Profile {
	return pprof2.RawFromProto(&profilev1.Profile{
		SampleType:  []*profilev1.ValueType{{Type: 1, Unit: 2}},
		Sample:      []*profilev1.Sample{{LocationId: []uint64{1}, Value: []int64{1}}},
		Mapping:     []*profilev1.Mapping{{Id: 1, HasFunctions: true}},
		Location:    []*profilev1.Location{{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}}},
		Function:    []*profilev1.Function{{Id: 1, Name: 3, SystemName: 3, Filename: 4}},
		StringTable: []string{"", "cpu", "nanoseconds", "main", "main.go"},
	})
}

// runPushBatchProbe pushes n distinct series through a probe distributor bounded
// to maxConcurrency and returns the probe for assertions.
func runPushBatchProbe(t *testing.T, maxConcurrency, n int) *probeSegmentWriter {
	t.Helper()
	probe := &probeSegmentWriter{}
	d := newProbeDistributor(t, probe, maxConcurrency)
	ctx := tenant.InjectTenantID(context.Background(), "user-1")
	err := d.PushBatch(ctx, &distributormodel.PushRequest{
		RawProfileType: distributormodel.RawProfileTypePPROF,
		Series:         probeProfileSeries(n),
	})
	require.NoError(t, err)
	return probe
}

// TestPushBatch_ConcurrencyCeiling: the fan-out never exceeds the configured
// bound, and every series is attempted.
func TestPushBatch_ConcurrencyCeiling(t *testing.T) {
	t.Parallel()
	const limit, n = 8, 50
	probe := runPushBatchProbe(t, limit, n)
	assert.LessOrEqual(t, probe.maxInFlight.Load(), int64(limit), "fan-out must never exceed the bound")
	assert.Equal(t, int64(n), probe.pushed.Load(), "all series attempted")
}

// TestPushBatch_LimitOneSerializes: a bound of 1 serializes the fan-out (kill
// switch) — at most one series is ever in flight.
func TestPushBatch_LimitOneSerializes(t *testing.T) {
	t.Parallel()
	const n = 10
	probe := runPushBatchProbe(t, 1, n)
	assert.LessOrEqual(t, probe.maxInFlight.Load(), int64(1), "limit=1 must serialize pushes")
	assert.Equal(t, int64(n), probe.pushed.Load(), "all series attempted")
}

// TestPushBatch_LimitZeroUnbounded: a bound of 0 is legacy/unbounded. The point
// is that a non-positive bound skips SetLimit rather than calling SetLimit(0),
// which would deadlock — so all N series must complete.
func TestPushBatch_LimitZeroUnbounded(t *testing.T) {
	t.Parallel()
	const n = 16
	probe := runPushBatchProbe(t, 0, n)
	assert.Equal(t, int64(n), probe.pushed.Load(), "all series must complete (no SetLimit(0) deadlock)")
}

// TestPushBatch_AggregatesAllErrors: every failing series is attempted and its
// error is aggregated into the multierror, each wrapped with its index;
// non-failing series still succeed.
func TestPushBatch_AggregatesAllErrors(t *testing.T) {
	t.Parallel()
	const n = 10
	failIdx := []int{2, 5, 8}
	probe := &probeSegmentWriter{failServices: map[string]bool{}}
	for _, i := range failIdx {
		probe.failServices[fmt.Sprintf("svc-%d", i)] = true
	}
	d := newProbeDistributor(t, probe, 4)
	ctx := tenant.InjectTenantID(context.Background(), "user-1")

	err := d.PushBatch(ctx, &distributormodel.PushRequest{
		RawProfileType: distributormodel.RawProfileTypePPROF,
		Series:         probeProfileSeries(n),
	})
	require.Error(t, err)
	msg := err.Error()
	for _, i := range failIdx {
		assert.Contains(t, msg, fmt.Sprintf("push series with index %d and", i), "aggregated error must include failing series %d", i)
		assert.Contains(t, msg, fmt.Sprintf("probe failure for svc-%d", i))
	}
	// Non-failing series must not surface as errors.
	assert.NotContains(t, msg, "push series with index 0 and")
	assert.NotContains(t, msg, "push series with index 9 and")
	// Every non-failing series was still pushed.
	assert.Equal(t, int64(n-len(failIdx)), probe.pushed.Load())
}

// TestPushBatch_PanicInOneSeriesDoesNotKillOthers: a panic in one series is
// recovered (util.RecoverPanic / router recovery) and surfaces as an aggregated
// error; the other series still complete.
func TestPushBatch_PanicInOneSeriesDoesNotKillOthers(t *testing.T) {
	t.Parallel()
	const n = 5
	probe := &probeSegmentWriter{panicServices: map[string]bool{"svc-1": true}}
	d := newProbeDistributor(t, probe, 4)
	ctx := tenant.InjectTenantID(context.Background(), "user-1")

	err := d.PushBatch(ctx, &distributormodel.PushRequest{
		RawProfileType: distributormodel.RawProfileTypePPROF,
		Series:         probeProfileSeries(n),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "push series with index 1 and")
	assert.Contains(t, err.Error(), "probe panic for svc-1")
	// The panicking series is recovered; the other four still complete.
	assert.Equal(t, int64(n-1), probe.pushed.Load())
}
