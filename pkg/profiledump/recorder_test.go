package profiledump

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

const (
	resultEnqueued = "enqueued"
	resultDropped  = "dropped"
)

var recorderNow = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

type policySet struct {
	mu       sync.RWMutex
	policies map[string]Policy
}

func (s *policySet) ProfileDebugDump(tenant string) Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policies[tenant]
}
func (s *policySet) set(tenant string, p Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[tenant] = p
}

type testClock struct{ nanos atomic.Int64 }

func (c *testClock) now() time.Time          { return recorderNow.Add(time.Duration(c.nanos.Load())) }
func (c *testClock) advance(d time.Duration) { c.nanos.Add(int64(d)) }

func recorderPolicy(t *testing.T, selector string, probability, rate float64) Policy {
	t.Helper()
	deadline := recorderNow.Add(time.Hour)
	p, err := (&TenantConfig{ActiveUntil: &deadline, Selector: &selector, Probability: &probability, MaxCapturesPerSecond: &rate}).Compile(DefaultConfig(), recorderNow)
	require.NoError(t, err)
	return p
}

func candidate(body []byte) Candidate {
	return Candidate{Metadata: NativeMetadata{SourceProtocol: SourceConnect, NativeFormat: FormatPprof, PayloadEncoding: "identity"}, Payload: body}
}

func recorderFixture(t *testing.T, cfg recorderFixtureConfig, upload UploadFunc, modify func(*Dependencies)) (*Recorder, *policySet, *testClock) {
	t.Helper()
	policies := &policySet{policies: map[string]Policy{"a": recorderPolicy(t, "{}", 1, 10), "b": recorderPolicy(t, "{}", 1, 10)}}
	clock := &testClock{}
	deps := Dependencies{Policies: policies, DistributorID: "distributor-test", Upload: upload, Registerer: prometheus.NewRegistry(), Now: clock.now}
	if modify != nil {
		modify(&deps)
	}
	r, err := NewRecorder(cfg.RecorderConfig, deps)
	require.NoError(t, err)
	r.queue = make(chan uploadItem, cfg.QueueCapacity)
	r.workers, r.tenantBurst, r.maxTenantLimiters = cfg.Workers, cfg.TenantBurst, cfg.MaxTenantLimiters
	r.process = rate.NewLimiter(rate.Limit(cfg.ProcessCapturesPerSecond), cfg.ProcessBurst)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), r))
	t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r)) })
	return r, policies, clock
}

// Test-only tuning keeps deterministic ownership and limiter scenarios small.
type recorderFixtureConfig struct {
	RecorderConfig
	QueueCapacity, Workers, TenantBurst, ProcessBurst, MaxTenantLimiters int
}

func recorderTestConfig() recorderFixtureConfig {
	c := recorderFixtureConfig{RecorderConfig: DefaultRecorderConfig(), MaxTenantLimiters: defaultMaxTenantLimiters}
	c.TenantBurst = 10000
	c.ProcessBurst = 10000
	c.QueueCapacity = 128
	c.Workers = 1
	return c
}
func discardUpload(_ context.Context, _, _ string, body io.Reader) error {
	_, err := io.Copy(io.Discard, body)
	return err
}
func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("synchronization timed out")
		var v T
		return v
	}
}
func retained(r *Recorder) int64 { r.mu.Lock(); defer r.mu.Unlock(); return r.retained }
func assertReleased(t *testing.T, r *Recorder) {
	t.Helper()
	require.Zero(t, retained(r))
	require.Zero(t, testutil.ToFloat64(r.metrics.retained))
	require.Zero(t, testutil.ToFloat64(r.metrics.queueItems))
	require.Zero(t, testutil.ToFloat64(r.metrics.queueBytes))
}

func TestRecorderSelection(t *testing.T) {
	for _, tc := range []struct {
		name                string
		policy              bool
		advance             time.Duration
		selector            string
		source              SourceProtocol
		labels              labels.Labels
		probability, random float64
		reason              DropReason
	}{
		{name: "absent", reason: DropDisabled},
		{name: "deadline", policy: true, advance: time.Hour, selector: "{}", probability: 1, reason: DropExpired},
		{name: "connect hit", policy: true, selector: `{service_name="checkout"}`, source: SourceConnect, labels: labels.FromStrings("service_name", "checkout"), probability: 1},
		{name: "empty labels still match", policy: true, selector: `{service_name="checkout"}`, source: SourceConnect, probability: 1, reason: DropSelector},
		{name: "partial accepted", policy: true, selector: "{}", probability: .5, random: .49},
		{name: "partial rejected", policy: true, selector: "{}", probability: .5, random: .5, reason: DropSampled},
		{name: "one skips random", policy: true, selector: "{}", probability: 1, random: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, policies, clock := recorderFixture(t, recorderTestConfig(), discardUpload, func(d *Dependencies) {
				d.Random = func() float64 {
					if tc.probability == 1 {
						t.Error("probability one should skip randomness")
					}
					return tc.random
				}
			})
			p := Policy{}
			if tc.policy {
				p = recorderPolicy(t, tc.selector, tc.probability, 10)
			}
			policies.set("a", p)
			clock.advance(tc.advance)
			c := candidate(nil)
			c.SelectorLabels = tc.labels
			if tc.source != "" {
				c.Metadata.SourceProtocol = tc.source
			}
			out := r.Capture(context.Background(), "a", c)
			require.Equal(t, tc.reason, out.Reason)
			require.Equal(t, tc.reason == "", out.Enqueued)
			result := resultEnqueued
			if tc.reason != "" {
				result = resultDropped
				require.Equal(t, 1., testutil.ToFloat64(r.metrics.dropped.WithLabelValues(metricSource(c.Metadata.SourceProtocol), string(tc.reason))))
				require.Zero(t, testutil.ToFloat64(r.metrics.bytes.WithLabelValues(metricSource(c.Metadata.SourceProtocol), result)))
			}
			require.Equal(t, 1., testutil.ToFloat64(r.metrics.candidates.WithLabelValues(metricSource(c.Metadata.SourceProtocol), result)))
			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
			assertReleased(t, r)
		})
	}
}

func TestRecorderDisabledNoPayloadAllocation(t *testing.T) {
	var nilRecorder *Recorder
	c := candidate(nil)
	require.Equal(t, DropDisabled, nilRecorder.Capture(context.Background(), "a", c).Reason)
	r, p, _ := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	p.set("a", Policy{})
	require.Zero(t, testing.AllocsPerRun(100, func() { r.Capture(context.Background(), "a", c) }))
	require.Zero(t, testing.AllocsPerRun(100, func() { nilRecorder.Capture(context.Background(), "a", c) }))
}

func TestRecorderRatesAndReload(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.TenantBurst = 1
	cfg.ProcessBurst = 2
	cfg.ProcessCapturesPerSecond = 1
	r, policies, clock := recorderFixture(t, cfg, discardUpload, nil)
	capture := func(tenant string) Outcome { return r.Capture(context.Background(), tenant, candidate([]byte("raw"))) }
	require.True(t, capture("a").Enqueued)
	policies.set("a", recorderPolicy(t, `{env!="dev"}`, 1, 5))
	require.Equal(t, DropTenantRate, capture("a").Reason, "reload must not replenish burst")
	require.True(t, capture("b").Enqueued)
	policies.set("c", recorderPolicy(t, "{}", 1, 10))
	require.Equal(t, DropProcessRate, capture("c").Reason)
	clock.advance(time.Second)
	require.True(t, capture("a").Enqueued)
	require.Equal(t, DropProcessRate, capture("b").Reason)
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	assertReleased(t, r)
}

func TestRecorderOwnedUploadAndRequestIsolation(t *testing.T) {
	started, proceed := make(chan struct{}), make(chan struct{})
	type uploaded struct {
		tenant, key string
		data        []byte
		span        oteltrace.SpanContext
		value       any
		err         error
	}
	result := make(chan uploaded, 2)
	type contextKey struct{}
	r, policies, clock := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, tenant, key string, body io.Reader) error {
		if strings.HasSuffix(key, ".pprof") {
			close(started)
		}
		<-proceed
		data, err := io.ReadAll(body)
		result <- uploaded{tenant, key, data, oteltrace.SpanContextFromContext(ctx), ctx.Value(contextKey{}), ctx.Err()}
		return err
	}, nil)
	body := []byte("native bytes")
	c := candidate(body)
	c.Metadata.Labels = map[string]string{"service_name": "original"}
	c.Metadata.PayloadEncoding = "gzip"
	span := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: oteltrace.TraceID{1}, SpanID: oteltrace.SpanID{2}, TraceFlags: oteltrace.FlagsSampled})
	ctx, cancel := context.WithCancel(context.WithValue(oteltrace.ContextWithSpanContext(context.Background(), span), contextKey{}, "request secret"))
	defer cancel()
	out := r.Capture(ctx, "a", c)
	require.True(t, out.Enqueued)
	await(t, started)
	cancel()
	copy(body, "MODIFIED!!!!")
	c.Metadata.Labels["service_name"] = "modified"
	c.Metadata.PayloadEncoding = "identity"
	policies.set("a", Policy{})
	require.Equal(t, DropDisabled, r.Capture(context.Background(), "a", candidate(nil)).Reason)
	clock.advance(time.Hour)
	require.Equal(t, DropExpired, r.Capture(context.Background(), "b", candidate(nil)).Reason)
	close(proceed)
	got := await(t, result)
	require.NoError(t, got.err)
	require.Nil(t, got.value)
	require.Equal(t, span, got.span)
	require.Equal(t, "a", got.tenant)
	require.Equal(t, out.ObjectKey, got.key)
	key, err := ParseNativeObjectKey(got.key)
	require.NoError(t, err)
	require.Equal(t, "a", key.TenantID)
	sidecar := await(t, result)
	require.Equal(t, key.MetadataKey, sidecar.key)
	require.Equal(t, span, sidecar.span)
	require.NoError(t, sidecar.err)
	require.Nil(t, sidecar.value)
	m, err := ReadNativeMetadata(bytes.NewReader(sidecar.data), sidecar.key)
	require.NoError(t, err)
	require.Equal(t, "native bytes", string(got.data))
	require.Equal(t, "original", m.Labels["service_name"])
	require.Equal(t, "gzip", m.PayloadEncoding)
	require.Equal(t, out.CaptureID, m.CaptureID)
	require.Equal(t, out.PolicyFingerprint, m.PolicyFingerprint)
	require.Equal(t, int64(len(got.data)+len(sidecar.data)), out.Size)
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	assertReleased(t, r)
}

func TestRecorderUploadError(t *testing.T) {
	r, _, _ := recorderFixture(t, recorderTestConfig(), func(context.Context, string, string, io.Reader) error {
		return errors.New("storage error")
	}, nil)
	require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	assertReleased(t, r)
	require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "error")))
	require.Zero(t, testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "uploaded")))
}

func TestRecorderGracefulDrain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, proceed := make(chan context.Context, 1), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		var calls atomic.Int64
		r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
			if calls.Add(1) == 1 {
				started <- ctx
				<-proceed
			}
			return ctx.Err()
		}, nil)
		t.Cleanup(unblock)
		require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
		uploadCtx := await(t, started)
		require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
		r.StopAsync()
		synctest.Wait()
		require.NoError(t, uploadCtx.Err(), "graceful draining must not cancel uploads")
		require.Equal(t, DropShutdown, r.Capture(context.Background(), "a", candidate(nil)).Reason)
		unblock()
		require.NoError(t, r.AwaitTerminated(context.Background()))
		require.Equal(t, int64(4), calls.Load())
		assertReleased(t, r)
	})
}

func TestRecorderAcceptedWorkDrainsAfterDeactivation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("expired=%t", expired), func(t *testing.T) {
			started, proceed := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(proceed) })
			var calls atomic.Int64
			r, policies, clock := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, body io.Reader) error {
				if calls.Add(1) == 1 {
					close(started)
				}
				select {
				case <-proceed:
				case <-ctx.Done():
					return ctx.Err()
				}
				_, err := io.Copy(io.Discard, body)
				return err
			}, nil)
			t.Cleanup(unblock)
			require.True(t, r.Capture(context.Background(), "a", candidate([]byte("uploading"))).Enqueued)
			await(t, started)
			require.True(t, r.Capture(context.Background(), "a", candidate([]byte("queued"))).Enqueued)
			reason := DropDisabled
			if expired {
				clock.advance(time.Hour)
				reason = DropExpired
			} else {
				policies.set("a", Policy{})
			}
			require.False(t, r.PolicyActive("a"))
			require.Equal(t, reason, r.Capture(context.Background(), "a", candidate(nil)).Reason)
			unblock()
			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
			require.Equal(t, int64(4), calls.Load())
			require.Equal(t, 2., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "success")))
			assertReleased(t, r)
		})
	}
}

func TestRecorderLimiterPruning(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.MaxTenantLimiters = 2
	cfg.TenantBurst = 1
	r, p, clock := recorderFixture(t, cfg, discardUpload, nil)
	capture := func(tenant string) Outcome { return r.Capture(context.Background(), tenant, candidate(nil)) }
	require.True(t, capture("a").Enqueued)
	require.True(t, capture("b").Enqueued)
	p.set("c", recorderPolicy(t, "{}", 1, 10))
	require.Equal(t, DropLimiterCapacity, capture("c").Reason)
	p.set("a", Policy{})
	require.True(t, capture("c").Enqueued, "capacity pressure prunes removed policies")
	require.Equal(t, DropTenantRate, capture("b").Reason, "pruning must preserve active token history")
	r.mu.Lock()
	require.Len(t, r.tenants, 2)
	require.NotContains(t, r.tenants, "a")
	r.mu.Unlock()
	clock.advance(time.Hour)
	deadline := clock.now().Add(time.Minute)
	probability := 1.0
	active, err := (&TenantConfig{ActiveUntil: &deadline, Probability: &probability}).Compile(DefaultConfig(), clock.now())
	require.NoError(t, err)
	p.set("d", active)
	require.True(t, capture("d").Enqueued, "capacity pressure prunes expired policies without a timer")
	r.mu.Lock()
	require.Len(t, r.tenants, 1)
	require.Contains(t, r.tenants, "d")
	r.mu.Unlock()
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	require.Empty(t, r.tenants)
}

func TestRecorderConcurrentReloadAndShutdown(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.Workers = 4
	cfg.QueueCapacity = 4
	r, p, clock := recorderFixture(t, cfg, discardUpload, nil)
	policy := recorderPolicy(t, `{env!="dev"}`, 1, 10)
	alternate := recorderPolicy(t, `{env="dev"}`, 1, 10)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			series := r.PrepareSeries("a", nil)
			for j := 0; j < 200; j++ {
				_ = r.PolicyActive("a")
				r.Capture(context.Background(), "a", candidate([]byte("native")))
				series.Capture(context.Background(), candidate([]byte("native")))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			p.set("a", alternate)
			p.set("a", Policy{})
			p.set("a", policy)
			clock.advance(time.Millisecond)
		}
	}()
	close(start)
	// A synchronized first admission ensures shutdown overlaps a running producer.
	r.Capture(context.Background(), "b", candidate(nil))
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	wg.Wait()
	assertReleased(t, r)
}

func TestRecorderConfigValidation(t *testing.T) {
	for _, mutate := range []func(*RecorderConfig){
		func(c *RecorderConfig) { c.MaxObjectBytes = 0 },
		func(c *RecorderConfig) { c.MaxObjectBytes = math.MaxInt64 },
		func(c *RecorderConfig) { c.MaxRetainedBytes = c.MaxObjectBytes },
		func(c *RecorderConfig) { c.ProcessCapturesPerSecond = math.Inf(1) },
		func(c *RecorderConfig) { c.UploadTimeout = 0 },
	} {
		c := DefaultRecorderConfig()
		mutate(&c)
		require.Error(t, c.Validate())
		r, err := NewRecorder(c, Dependencies{Policies: &policySet{}, Upload: discardUpload})
		require.Error(t, err)
		require.Nil(t, r)
	}
	_, err := NewRecorder(DefaultRecorderConfig(), Dependencies{})
	require.Error(t, err)
}

func BenchmarkRecorderDisabled(b *testing.B) {
	r := &Recorder{deps: Dependencies{Now: time.Now, Policies: &policySet{policies: map[string]Policy{}}}, metrics: newRecorderMetrics(nil)}
	c := candidate(nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Capture(context.Background(), "a", c)
	}
}

func TestRecorderPolicyActive(t *testing.T) {
	var disabled *Recorder
	require.False(t, disabled.PolicyActive("a"))
	cfg := recorderTestConfig()
	cfg.TenantBurst = 1
	r, policies, clock := recorderFixture(t, cfg, discardUpload, nil)
	require.False(t, r.PolicyActive("unknown"))
	for range 10 {
		require.True(t, r.PolicyActive("a"))
	}
	// Hints do not consume rate or memory admission.
	require.Zero(t, retained(r))
	require.True(t, r.Capture(context.Background(), "a", candidate([]byte("native"))).Enqueued)
	require.Equal(t, DropTenantRate, r.Capture(context.Background(), "a", candidate(nil)).Reason)
	// A positive hint grants no stale permission after removal or expiry.
	require.True(t, r.PolicyActive("a"))
	policies.set("a", Policy{})
	require.False(t, r.PolicyActive("a"))
	require.Equal(t, DropDisabled, r.Capture(context.Background(), "a", candidate(nil)).Reason)
	policies.set("a", recorderPolicy(t, "{}", 1, 10))
	require.True(t, r.PolicyActive("a"))
	clock.advance(time.Hour)
	require.False(t, r.PolicyActive("a"))
	require.Equal(t, DropExpired, r.Capture(context.Background(), "a", candidate(nil)).Reason)
}

func TestRecorderImplementationBounds(t *testing.T) {
	p := recorderPolicy(t, "{}", 1, 1)
	policies := &policySet{policies: map[string]Policy{}}
	r, err := NewRecorder(DefaultRecorderConfig(), Dependencies{Policies: policies, Upload: discardUpload})
	require.NoError(t, err)
	require.Equal(t, 16, cap(r.queue))
	require.Equal(t, 2, r.workers)
	require.Equal(t, 2, r.process.Burst())
	for i := range 1024 {
		tenant := fmt.Sprint(i)
		policies.set(tenant, p)
		require.Empty(t, r.admitRate(tenant, p, recorderNow.Add(time.Duration(i)*time.Second)))
		require.Equal(t, 1, r.tenants[tenant].Burst())
	}
	require.Len(t, r.tenants, 1024)
	require.Equal(t, DropLimiterCapacity, r.admitRate("overflow", p, recorderNow.Add(1024*time.Second)))
	require.Len(t, r.tenants, 1024)
	policies.set("0", Policy{})
	require.Empty(t, r.admitRate("overflow", p, recorderNow.Add(1024*time.Second)))
	require.Len(t, r.tenants, 1024)
	require.NotContains(t, r.tenants, "0")
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
}
