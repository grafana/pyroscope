package profiledump

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

const resultEnqueued = "enqueued"

var recorderNow = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

type policySet struct {
	mu       sync.RWMutex
	policies map[string]Policy
}

func (s *policySet) ProfileDump(tenant string) Policy {
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

func recorderPolicy(t *testing.T, selector string, probability float64) Policy {
	t.Helper()
	deadline := recorderNow.Add(time.Hour)
	p, err := (&TenantConfig{ActiveUntil: &deadline, Selector: &selector, Probability: &probability}).Compile(recorderNow)
	require.NoError(t, err)
	require.True(t, p.ActiveAt(recorderNow))
	return p
}

func captureMetadata() Metadata {
	return Metadata{SourceProtocol: sourceConnect, Format: formatPprof}
}

func recorderFixture(t *testing.T, cfg recorderFixtureConfig, upload UploadFunc) (*Recorder, *policySet, *testClock) {
	t.Helper()
	policies := &policySet{policies: map[string]Policy{"a": recorderPolicy(t, "{}", 1), "b": recorderPolicy(t, "{}", 1)}}
	clock := &testClock{}
	r, err := NewRecorder(cfg.RecorderConfig, policies, upload, "distributor-test", log.NewNopLogger(), prometheus.NewRegistry())
	require.NoError(t, err)
	r.now = clock.now
	r.queue = make(chan uploadItem, cfg.QueueCapacity)
	r.workers = cfg.Workers
	r.process = rate.NewLimiter(processCapturesPerSecond, cfg.ProcessBurst)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), r))
	t.Cleanup(func() { require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r)) })
	return r, policies, clock
}

// Test-only tuning keeps deterministic ownership and limiter scenarios small.
type recorderFixtureConfig struct {
	RecorderConfig
	QueueCapacity, Workers, ProcessBurst int
}

func recorderTestConfig() recorderFixtureConfig {
	c := recorderFixtureConfig{RecorderConfig: DefaultRecorderConfig()}
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

func awaitUploads(t *testing.T, r *Recorder, count float64) {
	t.Helper()
	require.Eventually(t, func() bool {
		var total float64
		for _, result := range []string{"success", "error", "timeout"} {
			total += testutil.ToFloat64(r.metrics.uploads.WithLabelValues(result))
		}
		return total == count
	}, 5*time.Second, time.Millisecond)
}

func TestRecorderRatesAndReload(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.ProcessBurst = defaultProcessBurst
	r, policies, clock := recorderFixture(t, cfg, discardUpload)
	series := &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{{RawProfile: []byte("raw")}}}
	capture := func(tenant string) { r.CaptureSeries(context.Background(), tenant, series) }
	// A batch can use the full burst, with one token consumed per profile.
	batch := &pushv1.RawProfileSeries{}
	for range 10 {
		batch.Samples = append(batch.Samples, &pushv1.RawSample{RawProfile: []byte("raw")})
	}
	r.CaptureSeries(context.Background(), "a", batch)
	policies.set("a", recorderPolicy(t, `{env!="dev"}`, 1))
	capture("a") // Reload must not replenish tokens.
	capture("b") // Other tenants share the exhausted process allowance.
	require.Equal(t, 10., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
	require.Equal(t, 2., testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(dropProcessRate))))
	clock.advance(99 * time.Millisecond)
	capture("b")
	require.Equal(t, 10., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
	clock.advance(time.Millisecond)
	capture("b")
	capture("a")
	require.Equal(t, 11., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
	require.Equal(t, 4., testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(dropProcessRate))))
	clock.advance(100 * time.Millisecond)
	// Admission must use current time even when the series capture timestamp is stale.
	reads := 0
	r.now = func() time.Time {
		reads++
		if reads == 1 {
			return recorderNow
		}
		return clock.now()
	}
	capture("a")
	require.Equal(t, 12., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
	capture("b")
	require.Equal(t, 5., testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(dropProcessRate))))
}

func TestRecorderOwnedUploadAndRequestIsolation(t *testing.T) {
	started, proceed := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(proceed) })
	type uploaded struct {
		tenant string
		data   []byte
		span   oteltrace.SpanContext
		value  any
		err    error
	}
	result := make(chan uploaded, 2)
	type contextKey struct{}
	r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, tenant, key string, body io.Reader) error {
		if strings.HasSuffix(key, ".pprof") {
			close(started)
		}
		<-proceed
		data, err := io.ReadAll(body)
		result <- uploaded{tenant, data, oteltrace.SpanContextFromContext(ctx), ctx.Value(contextKey{}), ctx.Err()}
		return err
	})
	t.Cleanup(unblock)
	body := []byte("payload bytes")
	metadata, payload := captureMetadata(), body
	metadata.Labels = []*typesv1.LabelPair{{Name: "service_name", Value: "original"}}
	span := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: oteltrace.TraceID{1}, SpanID: oteltrace.SpanID{2}, TraceFlags: oteltrace.FlagsSampled})
	ctx, cancel := context.WithCancel(context.WithValue(oteltrace.ContextWithSpanContext(context.Background(), span), contextKey{}, "request secret"))
	defer cancel()
	out := r.capture(ctx, "a", metadata, payload, r.now())
	require.Empty(t, out)
	await(t, started)
	cancel()
	copy(body, "MODIFIED!!!!")
	metadata.Labels[0].Value = "modified"
	unblock()
	got := await(t, result)
	require.NoError(t, got.err)
	require.Nil(t, got.value)
	require.Equal(t, span, got.span)
	require.Equal(t, "a", got.tenant)
	sidecar := await(t, result)
	require.Equal(t, span, sidecar.span)
	require.NoError(t, sidecar.err)
	require.Nil(t, sidecar.value)
	var m Metadata
	require.NoError(t, json.Unmarshal(sidecar.data, &m))
	require.Equal(t, "payload bytes", string(got.data))
	require.Equal(t, "original", m.Labels[0].Value)
}

func TestRecorderConcurrentReloadAndShutdown(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.Workers = 4
	cfg.QueueCapacity = 4
	r, p, clock := recorderFixture(t, cfg, discardUpload)
	policy := recorderPolicy(t, `{env!="dev"}`, 1)
	alternate := recorderPolicy(t, `{env="dev"}`, 1)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 200; j++ {
				r.CaptureSeries(context.Background(), "a", &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{{RawProfile: []byte("payload")}}})
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
	// Exercise shutdown while the producer goroutines are running.
	r.capture(context.Background(), "b", captureMetadata(), nil, r.now())
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), r))
	wg.Wait()
}

func TestRecorderConfigValidation(t *testing.T) {
	r, err := NewRecorder(RecorderConfig{}, &policySet{}, discardUpload, "test", log.NewNopLogger(), nil)
	require.Error(t, err)
	require.Nil(t, r)
	_, err = NewRecorder(DefaultRecorderConfig(), nil, discardUpload, "test", log.NewNopLogger(), nil)
	require.Error(t, err)
	_, err = NewRecorder(DefaultRecorderConfig(), &policySet{}, nil, "test", log.NewNopLogger(), nil)
	require.Error(t, err)
}
