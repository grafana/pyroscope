package profiledump

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
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
	return Candidate{Metadata: Metadata{SourceProtocol: SourceConnect, NativeFormat: FormatPprof, Stored: Representation{ContentType: "application/octet-stream", Encoding: "identity", Syntax: "binary"}}, Payload: BytesPayload(body)}
}

func recorderFixture(t *testing.T, cfg RecorderConfig, upload UploadFunc, modify func(*Dependencies)) (*Recorder, *policySet, *testClock) {
	t.Helper()
	policies := &policySet{policies: map[string]Policy{"a": recorderPolicy(t, "{}", 1, 10), "b": recorderPolicy(t, "{}", 1, 10)}}
	clock := &testClock{}
	deps := Dependencies{Policies: policies, DistributorID: "distributor-test", Upload: upload, Registerer: prometheus.NewRegistry(), Now: clock.now}
	if modify != nil {
		modify(&deps)
	}
	r, err := NewRecorder(cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { r.cancel(); _ = r.Shutdown(context.Background()); await(t, r.Done()) })
	return r, policies, clock
}
func recorderTestConfig() RecorderConfig {
	c := DefaultRecorderConfig()
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

type funcPayload struct {
	size, scratch int64
	sizeErr       error
	write         func(context.Context, []byte, []byte) (int, error)
	sizeFunc      func() (int64, int64, error)
	sizes, writes atomic.Int64
}

func (p *funcPayload) Size() (int64, int64, error) {
	p.sizes.Add(1)
	if p.sizeFunc != nil {
		return p.sizeFunc()
	}
	return p.size, p.scratch, p.sizeErr
}
func (p *funcPayload) Write(ctx context.Context, dst, scratch []byte) (int, error) {
	p.writes.Add(1)
	if p.write != nil {
		return p.write(ctx, dst, scratch)
	}
	clear(dst)
	return len(dst), nil
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
		{name: "legacy miss", policy: true, selector: `{service_name="checkout"}`, source: SourceIngest, probability: 1, reason: DropSelector},
		{name: "OTLP HTTP bypass", policy: true, selector: `{service_name="never"}`, source: SourceOTLPHTTP, probability: 1},
		{name: "OTLP gRPC bypass", policy: true, selector: `{service_name="never"}`, source: SourceOTLPGRPC, probability: 1},
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
			payload := &funcPayload{size: 4}
			c := candidate(nil)
			c.Payload = payload
			c.SelectorLabels = tc.labels
			if tc.source != "" {
				c.Metadata.SourceProtocol = tc.source
			}
			spans := 0
			c.StartSpan = func(ctx context.Context) (context.Context, func(Outcome)) { spans++; return ctx, func(Outcome) {} }
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
			if tc.reason != "" {
				require.Zero(t, payload.sizes.Load())
				require.Zero(t, payload.writes.Load())
				require.Zero(t, spans)
			} else {
				require.Equal(t, 1, spans)
			}
			require.NoError(t, r.Shutdown(context.Background()))
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
	require.NoError(t, r.Shutdown(context.Background()))
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
	result := make(chan uploaded, 1)
	type contextKey struct{}
	r, policies, clock := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, tenant, key string, body io.Reader) error {
		close(started)
		<-proceed
		data, err := io.ReadAll(body)
		result <- uploaded{tenant, key, data, oteltrace.SpanContextFromContext(ctx), ctx.Value(contextKey{}), ctx.Err()}
		return err
	}, nil)
	body := []byte("native bytes")
	c := candidate(body)
	c.Metadata.Labels = map[string]string{"service_name": "original"}
	c.Metadata.Incoming = &Representation{ContentType: "application/octet-stream", Encoding: "gzip", Syntax: "binary"}
	span := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: oteltrace.TraceID{1}, SpanID: oteltrace.SpanID{2}, TraceFlags: oteltrace.FlagsSampled})
	ctx, cancel := context.WithCancel(context.WithValue(oteltrace.ContextWithSpanContext(context.Background(), span), contextKey{}, "request secret"))
	defer cancel()
	out := r.Capture(ctx, "a", c)
	require.True(t, out.Enqueued)
	await(t, started)
	cancel()
	copy(body, "MODIFIED!!!!")
	c.Metadata.Labels["service_name"] = "modified"
	c.Metadata.Incoming.Encoding = "identity"
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
	key, err := ParseObjectKey(got.key)
	require.NoError(t, err)
	require.Equal(t, "a", key.TenantID)
	var restored bytes.Buffer
	m, err := Decode(bytes.NewReader(got.data), &restored, r.cfg.MaxObjectBytes)
	require.NoError(t, err)
	require.Equal(t, "native bytes", restored.String())
	require.Equal(t, "original", m.Labels["service_name"])
	require.Equal(t, "gzip", m.Incoming.Encoding)
	require.Equal(t, out.CaptureID, m.CaptureID)
	require.Equal(t, out.PolicyFingerprint, m.PolicyFingerprint)
	require.Equal(t, int64(len(got.data)), out.Size)
	require.NoError(t, r.Shutdown(context.Background()))
	assertReleased(t, r)
}

func TestRecorderBoundsAndSerialization(t *testing.T) {
	for _, tc := range []struct {
		name              string
		size, scratch     int64
		sizeErr, writeErr error
		reason            DropReason
		writes            int64
	}{
		{name: "object including envelope too big", size: 1024, reason: DropTooLarge},
		{name: "payload overflow", size: math.MaxInt64, reason: DropTooLarge},
		{name: "scratch budget", size: 1, scratch: math.MaxInt64, reason: DropByteBudget},
		{name: "size error", sizeErr: errors.New("size"), reason: DropSerialization},
		{name: "negative size", size: -1, reason: DropSerialization},
		{name: "negative scratch", size: 1, scratch: -1, reason: DropSerialization},
		{name: "write error", size: 10, scratch: 100, writeErr: errors.New("write"), reason: DropSerialization, writes: 1},
		{name: "success", size: 10, scratch: 100, writes: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := recorderTestConfig()
			cfg.MaxObjectBytes = 1024
			cfg.MaxRetainedBytes = 1024 + encodingReservation
			r, _, _ := recorderFixture(t, cfg, discardUpload, nil)
			p := &funcPayload{size: tc.size, scratch: tc.scratch, sizeErr: tc.sizeErr, write: func(_ context.Context, dst, scratch []byte) (int, error) {
				require.Equal(t, tc.size, int64(len(dst)))
				require.Equal(t, tc.scratch, int64(len(scratch)))
				require.LessOrEqual(t, retained(r), cfg.MaxRetainedBytes)
				require.GreaterOrEqual(t, retained(r), int64(len(dst)+len(scratch))+encodingReservation)
				clear(dst)
				return len(dst), tc.writeErr
			}}
			c := candidate(nil)
			c.Payload = p
			out := r.Capture(context.Background(), "a", c)
			require.Equal(t, tc.reason, out.Reason)
			require.Equal(t, tc.writes, p.writes.Load())
			require.NoError(t, r.Shutdown(context.Background()))
			assertReleased(t, r)
		})
	}
}

func TestRecorderExactObjectBoundary(t *testing.T) {
	r, _, _ := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	c := candidate([]byte("payload"))
	out := r.Capture(context.Background(), "a", c)
	require.True(t, out.Enqueued)
	for _, delta := range []int64{0, -1} {
		cfg := recorderTestConfig()
		cfg.MaxObjectBytes = out.Size + delta
		rr, _, _ := recorderFixture(t, cfg, discardUpload, nil)
		got := rr.Capture(context.Background(), "a", c)
		require.Equal(t, delta == 0, got.Enqueued)
		if delta < 0 {
			require.Equal(t, DropTooLarge, got.Reason)
		}
	}
}

func TestRecorderSaturationAndRetainedBudget(t *testing.T) {
	for _, budget := range []bool{false, true} {
		t.Run(fmt.Sprint("budget=", budget), func(t *testing.T) {
			cfg := recorderTestConfig()
			cfg.QueueCapacity = 1
			cfg.MaxObjectBytes = 1024
			if budget {
				cfg.MaxRetainedBytes = 2 * (encodingReservation + 1024)
			}
			started, proceed := make(chan struct{}), make(chan struct{})
			var once sync.Once
			r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, _, _ string, _ io.Reader) error {
				once.Do(func() { close(started) })
				select {
				case <-proceed:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}, nil)
			c := candidate([]byte("owned"))
			first := r.Capture(context.Background(), "a", c)
			require.True(t, first.Enqueued)
			await(t, started)
			require.True(t, r.Capture(context.Background(), "a", c).Enqueued)
			reserved := retained(r)
			require.Equal(t, 2*(first.Size+encodingReservation), reserved)
			reason := DropQueueFull
			if budget {
				reason = DropByteBudget
			}
			for i := 0; i < 100; i++ {
				require.Equal(t, reason, r.Capture(context.Background(), "a", c).Reason)
				require.Equal(t, reserved, retained(r), "rejection must release exactly once")
			}
			require.Equal(t, 1., testutil.ToFloat64(r.metrics.queueItems))
			require.Equal(t, float64(first.Size), testutil.ToFloat64(r.metrics.queueBytes))
			close(proceed)
			require.NoError(t, r.Shutdown(context.Background()))
			assertReleased(t, r)
		})
	}
}

func TestRecorderUploadErrorAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint("timeout=", timeout), func(t *testing.T) {
			started := make(chan context.CancelFunc, 1)
			r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
				if timeout {
					<-ctx.Done()
					return ctx.Err()
				}
				return errors.New("storage error")
			}, func(d *Dependencies) {
				d.WithTimeout = func(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
					if duration != DefaultRecorderConfig().UploadTimeout {
						return context.WithTimeout(ctx, duration)
					}
					ctx, cancel := context.WithCancel(ctx)
					started <- cancel
					return ctx, cancel
				}
			})
			require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
			cancel := await(t, started)
			if timeout {
				cancel()
			}
			require.NoError(t, r.Shutdown(context.Background()))
			assertReleased(t, r)
			require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploadErrors.WithLabelValues("connect")))
			require.Zero(t, testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "uploaded")))
		})
	}
}

func TestRecorderGracefulDrain(t *testing.T) {
	started, proceed, draining := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
		if calls.Add(1) == 1 {
			close(started)
			<-proceed
		}
		return ctx.Err()
	}, func(d *Dependencies) {
		d.WithTimeout = func(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
			if duration == DefaultRecorderConfig().ShutdownDrain {
				select {
				case <-draining:
				default:
					close(draining)
				}
			}
			return context.WithTimeout(ctx, duration)
		}
	})
	require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	await(t, started)
	require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	stopped := make(chan error, 1)
	go func() { stopped <- r.Shutdown(context.Background()) }()
	await(t, draining)
	require.NoError(t, r.ctx.Err(), "graceful draining must not cancel uploads")
	require.Equal(t, DropShutdown, r.Capture(context.Background(), "a", candidate(nil)).Reason)
	close(proceed)
	require.NoError(t, await(t, stopped))
	require.Equal(t, int64(2), calls.Load())
	assertReleased(t, r)
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
			require.NoError(t, r.Shutdown(context.Background()))
			require.Equal(t, int64(2), calls.Load())
			require.Equal(t, 2., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "success")))
			assertReleased(t, r)
		})
	}
}

func TestRecorderForcedShutdownWithPreparation(t *testing.T) {
	uploadStarted, preparing := make(chan struct{}), make(chan struct{})
	timeout := make(chan context.CancelFunc, 1)
	var calls atomic.Int64
	r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
		calls.Add(1)
		close(uploadStarted)
		<-ctx.Done()
		return ctx.Err()
	}, func(d *Dependencies) {
		d.WithTimeout = func(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
			if duration != DefaultRecorderConfig().ShutdownDrain {
				return context.WithTimeout(ctx, duration)
			}
			ctx, cancel := context.WithCancel(ctx)
			select {
			case timeout <- cancel:
			default:
			}
			return ctx, cancel
		}
	})
	require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	await(t, uploadStarted)
	require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	c := candidate(nil)
	c.Payload = &funcPayload{size: 10, scratch: 100, write: func(ctx context.Context, _, _ []byte) (int, error) {
		close(preparing)
		<-ctx.Done()
		return 0, ctx.Err()
	}}
	captureDone := make(chan Outcome, 1)
	go func() { captureDone <- r.Capture(context.Background(), "a", c) }()
	await(t, preparing)
	require.Greater(t, retained(r), int64(3)*encodingReservation)
	stopped := make(chan error, 1)
	go func() { stopped <- r.Shutdown(context.Background()) }()
	cancel := await(t, timeout)
	require.NoError(t, r.ctx.Err())
	cancel()
	require.ErrorIs(t, await(t, stopped), context.Canceled)
	require.Equal(t, DropSerialization, await(t, captureDone).Reason)
	await(t, r.Done())
	require.Equal(t, int64(1), calls.Load())
	assertReleased(t, r)
}

// Done must wait for shutdown's last reservation after all workers exit.
func TestRecorderDoneWaitsForForcedShutdownReservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		uploadStarted, finishUpload := make(chan struct{}), make(chan struct{})
		drainOwnsItem, finishDrain := make(chan struct{}), make(chan struct{})
		releaseUpload := sync.OnceFunc(func() { close(finishUpload) })
		releaseDrain := sync.OnceFunc(func() { close(finishDrain) })
		r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
			close(uploadStarted)
			<-finishUpload
			return ctx.Err()
		}, nil)
		// Unblock both callbacks even if an assertion fails, before fixture cleanup.
		t.Cleanup(releaseUpload)
		t.Cleanup(releaseDrain)

		// Pause between dequeue and release without holding r.mu.
		r.metrics.dropped = prometheus.V2.NewCounterVec(prometheus.CounterVecOpts{
			CounterOpts: prometheus.CounterOpts{Name: "test_shutdown_dropped_total", Help: "Synchronizes forced shutdown."},
			VariableLabels: prometheus.ConstrainedLabels{
				{Name: "source", Constraint: func(source string) string {
					close(drainOwnsItem)
					<-finishDrain
					return source
				}},
				{Name: "reason"},
			},
		})
		require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
		await(t, uploadStarted)
		queued := r.Capture(context.Background(), "a", candidate(nil))
		require.True(t, queued.Enqueued)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		forcedStop := make(chan error, 1)
		go func() { forcedStop <- r.Shutdown(ctx) }()
		await(t, drainOwnsItem)
		releaseUpload()
		synctest.Wait()

		// Shutdown still owns the final capture after all workers exit.
		r.mu.Lock()
		workers, preparing := r.workers, r.preparing
		r.mu.Unlock()
		require.Zero(t, workers)
		require.Zero(t, preparing)
		require.Equal(t, queued.Size+encodingReservation, retained(r))
		select {
		case <-r.Done():
			t.Fatal("Done closed while forced shutdown still owns a reservation")
		default:
		}

		concurrentStop := make(chan error, 1)
		go func() { concurrentStop <- r.Shutdown(context.Background()) }()
		synctest.Wait()
		select {
		case err := <-concurrentStop:
			t.Fatalf("concurrent Shutdown returned before final release: %v", err)
		default:
		}

		releaseDrain()
		await(t, r.Done())
		assertReleased(t, r)
		require.ErrorIs(t, await(t, forcedStop), context.Canceled)
		require.NoError(t, await(t, concurrentStop))
	})
}

func TestRecorderPreparingDuringGracefulStop(t *testing.T) {
	preparing, proceed := make(chan struct{}), make(chan struct{})
	draining := make(chan struct{}, 1)
	r, _, _ := recorderFixture(t, recorderTestConfig(), discardUpload, func(d *Dependencies) {
		d.WithTimeout = func(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
			if d == DefaultRecorderConfig().ShutdownDrain {
				select {
				case draining <- struct{}{}:
				default:
				}
			}
			return context.WithTimeout(ctx, d)
		}
	})
	c := candidate(nil)
	c.Payload = &funcPayload{size: 1, write: func(_ context.Context, dst, _ []byte) (int, error) {
		close(preparing)
		<-proceed
		clear(dst)
		return len(dst), nil
	}}
	captured := make(chan Outcome, 1)
	go func() { captured <- r.Capture(context.Background(), "a", c) }()
	await(t, preparing)
	stopped := make(chan error, 1)
	go func() { stopped <- r.Shutdown(context.Background()) }()
	await(t, draining)
	close(proceed)
	require.Equal(t, DropShutdown, await(t, captured).Reason)
	require.NoError(t, await(t, stopped))
	assertReleased(t, r)
}

func TestRecorderLimiterPruning(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.MaxTenantLimiters = 2
	r, p, clock := recorderFixture(t, cfg, discardUpload, nil)
	c := candidate(nil)
	require.True(t, r.Capture(context.Background(), "a", c).Enqueued)
	require.True(t, r.Capture(context.Background(), "b", c).Enqueued)
	p.set("c", recorderPolicy(t, "{}", 1, 10))
	require.Equal(t, DropLimiterCapacity, r.Capture(context.Background(), "c", c).Reason)
	p.set("a", Policy{})
	require.True(t, r.Capture(context.Background(), "c", c).Enqueued, "capacity pressure prunes removed policies")
	clock.advance(time.Hour)
	r.mu.Lock()
	r.pruneLocked(clock.now())
	n := len(r.tenants)
	r.mu.Unlock()
	require.Zero(t, n)
}

func TestRecorderConcurrentReloadAndShutdown(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.Workers = 4
	cfg.QueueCapacity = 4
	r, p, clock := recorderFixture(t, cfg, discardUpload, nil)
	policy := recorderPolicy(t, "{}", 1, 10)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 200; j++ {
				_ = r.PolicyActive("a")
				r.Capture(context.Background(), "a", candidate([]byte("native")))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			p.set("a", Policy{})
			p.set("a", policy)
			clock.advance(time.Millisecond)
		}
	}()
	close(start)
	// A synchronized first admission ensures shutdown overlaps a running producer.
	r.Capture(context.Background(), "b", candidate(nil))
	require.NoError(t, r.Shutdown(context.Background()))
	wg.Wait()
	assertReleased(t, r)
}

func TestRecorderMetricsAndSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	reg := prometheus.NewRegistry()
	uploadStarted, releaseUpload := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(releaseUpload) })
	r, p, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
		close(uploadStarted)
		<-releaseUpload
		return ctx.Err()
	}, func(d *Dependencies) { d.Registerer = reg })
	t.Cleanup(unblock)
	c := candidate([]byte("data"))
	c.StartSpan = CaptureSpan(provider.Tracer("test"))
	out := r.Capture(context.Background(), "a", c)
	require.True(t, out.Enqueued)
	await(t, uploadStarted)
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, "Profile.Capture", spans[0].Name)
	require.Equal(t, out.SpanAttributes(), spans[0].Attributes)
	require.False(t, spans[0].EndTime.IsZero(), "capture span must end before upload completion is known")
	require.Equal(t, "distributor-test", out.DistributorID)
	require.Equal(t, SourceConnect, out.SourceProtocol)
	unblock()
	p.set("a", Policy{})
	r.Capture(context.Background(), "a", c)
	require.Len(t, exporter.GetSpans(), 1)
	c.Metadata.SourceProtocol = SourceProtocol("arbitrary-attacker-value")
	r.Capture(context.Background(), "a", c)
	require.NoError(t, r.Shutdown(context.Background()))
	assertReleased(t, r)
	require.Equal(t, 1., testutil.ToFloat64(r.metrics.candidates.WithLabelValues("connect", resultEnqueued)))
	require.Equal(t, float64(out.Size), testutil.ToFloat64(r.metrics.bytes.WithLabelValues("connect", "uploaded")))
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		for _, m := range f.Metric {
			for _, l := range m.Label {
				require.Contains(t, []string{"source", "result", "reason"}, l.GetName())
				require.NotEqual(t, "arbitrary-attacker-value", l.GetValue())
			}
		}
	}
}

func TestEncodedMetadataSizeParity(t *testing.T) {
	for _, s := range []string{"plain", `quotes"and\slashes`, "<&>", "line\u2028paragraph\u2029日本語", strings.Repeat("&", 1024)} {
		for _, optional := range []bool{false, true} {
			m := testMetadata(123456789)
			m.Labels = map[string]string{"label": s}
			m.OriginalProfileID = s
			if !optional {
				m.Labels = nil
				m.Incoming = nil
				m.OriginalProfileID = ""
			}
			require.NoError(t, m.Validate())
			b, err := json.Marshal(m)
			require.NoError(t, err)
			require.Equal(t, int64(len(b)), encodedMetadataSize(m))
		}
	}
}

func TestRecorderPrunesWithoutTraffic(t *testing.T) {
	ticks := make(chan time.Time)
	r, p, clock := recorderFixture(t, recorderTestConfig(), discardUpload, func(d *Dependencies) { d.PruneTicks = ticks })
	require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	p.set("a", Policy{})
	clock.advance(time.Minute)
	ticks <- clock.now()
	// Receiving the next tick establishes that the first sweep finished.
	ticks <- clock.now()
	r.mu.Lock()
	n := len(r.tenants)
	r.mu.Unlock()
	require.Zero(t, n)
}

func TestRecorderForcedShutdownKeepsExecutingBufferCharged(t *testing.T) {
	for _, duringUpload := range []bool{false, true} {
		t.Run(fmt.Sprint("upload=", duringUpload), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
				if duringUpload {
					close(started)
					<-release
				}
				return ctx.Err()
			}, nil)
			c := candidate(nil)
			if !duringUpload {
				c.Payload = &funcPayload{size: 32, write: func(ctx context.Context, _, _ []byte) (int, error) { close(started); <-release; return 0, ctx.Err() }}
			}
			captured := make(chan Outcome, 1)
			go func() { captured <- r.Capture(context.Background(), "a", c) }()
			await(t, started)
			before := retained(r)
			require.Positive(t, before)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			require.ErrorIs(t, r.Shutdown(ctx), context.Canceled)
			require.Equal(t, before, retained(r), "cannot release memory still in use by a callback")
			close(release)
			await(t, captured)
			await(t, r.Done())
			assertReleased(t, r)
		})
	}
}

func TestRecorderConfigValidation(t *testing.T) {
	for _, mutate := range []func(*RecorderConfig){
		func(c *RecorderConfig) { c.MaxObjectBytes = 0 },
		func(c *RecorderConfig) { c.MaxObjectBytes = math.MaxInt64 },
		func(c *RecorderConfig) { c.MaxRetainedBytes = c.MaxObjectBytes },
		func(c *RecorderConfig) { c.QueueCapacity = 0 },
		func(c *RecorderConfig) { c.Workers = -1 },
		func(c *RecorderConfig) { c.TenantBurst = 0 },
		func(c *RecorderConfig) { c.ProcessBurst = 0 },
		func(c *RecorderConfig) { c.ProcessCapturesPerSecond = math.Inf(1) },
		func(c *RecorderConfig) { c.UploadTimeout = 0 },
		func(c *RecorderConfig) { c.ShutdownDrain = 0 },
		func(c *RecorderConfig) { c.LimiterPruneInterval = 0 },
		func(c *RecorderConfig) { c.MaxTenantLimiters = 0 },
	} {
		c := DefaultRecorderConfig()
		mutate(&c)
		require.Error(t, c.Validate())
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
