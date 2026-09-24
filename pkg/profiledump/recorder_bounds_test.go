package profiledump

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestRecorderExactObjectAndMemory(t *testing.T) {
	// ULIDs and fingerprints have fixed lengths. Compute the boundary with the
	// accepted codec, including escaped labels and every recorder-owned field.
	c := candidate([]byte{0, 0xff, 0x1f, 0x8b})
	c.Metadata.Labels = map[string]string{"service_name": `<>&"`}
	m := c.Metadata
	m.SchemaVersion, m.CapturedAt, m.TenantID = Version, recorderNow, "a"
	m.DistributorID, m.ActivationSource = "distributor-test", ActivationRuntimeOverride
	m.PolicyFingerprint = recorderPolicy(t, "{}", 1, 10).Fingerprint()
	_, id, err := NewObjectKey("a", recorderNow, FormatPprof)
	require.NoError(t, err)
	m.CaptureID, m.PayloadSize = id.String(), int64(len(c.Payload))
	metadata, err := json.Marshal(m)
	require.NoError(t, err)
	objectSize := int64(HeaderSize + len(metadata) + len(c.Payload))
	for _, delta := range []int64{-1, 0, 1} {
		t.Run(string(rune('1'+delta)), func(t *testing.T) {
			cfg := recorderTestConfig()
			cfg.MaxObjectBytes = objectSize + delta
			started, proceed := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(proceed) })
			r, _, _ := recorderFixture(t, cfg, func(_ context.Context, _, _ string, body io.Reader) error {
				close(started)
				<-proceed
				var restored bytes.Buffer
				decoded, err := Decode(body, &restored, cfg.MaxObjectBytes)
				if err == nil && (!bytes.Equal(c.Payload, restored.Bytes()) || decoded.Labels["service_name"] != m.Labels["service_name"]) {
					t.Error("uploaded capture differs from candidate")
				}
				return err
			}, nil)
			t.Cleanup(unblock)
			out := r.Capture(context.Background(), "a", c)
			if delta < 0 {
				require.Equal(t, DropTooLarge, out.Reason)
				require.Zero(t, retained(r))
			} else {
				require.True(t, out.Enqueued)
				await(t, started)
				require.Equal(t, objectSize, out.Size)
				// The marshal result overlaps the complete object during construction.
				require.Equal(t, objectSize+int64(cap(metadata))+itemReservation, retained(r))
			}
			unblock()
			require.NoError(t, r.Shutdown(context.Background()))
			assertReleased(t, r)
		})
	}
}

func TestRecorderQueueAndByteBudget(t *testing.T) {
	for _, budget := range []bool{false, true} {
		t.Run(map[bool]string{false: "queue", true: "bytes"}[budget], func(t *testing.T) {
			cfg := recorderTestConfig()
			cfg.QueueCapacity = 1
			cfg.MaxObjectBytes = 4 << 20
			cfg.MaxRetainedBytes = cfg.MaxObjectBytes + MaxMetadataSize + itemReservation
			body := []byte("raw")
			if budget {
				body = make([]byte, 3<<20)
			}
			started, proceed := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(proceed) })
			var calls atomic.Int64
			r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, tenant, key string, body io.Reader) error {
				if calls.Add(1) == 1 {
					close(started)
				}
				<-proceed
				return discardUpload(ctx, tenant, key, body)
			}, nil)
			t.Cleanup(unblock)
			require.True(t, r.Capture(context.Background(), "a", candidate(body)).Enqueued)
			await(t, started)
			if !budget {
				require.True(t, r.Capture(context.Background(), "a", candidate(body)).Enqueued)
			}
			before := retained(r)
			want := DropQueueFull
			if budget {
				want = DropByteBudget
			}
			var allocationBefore, allocationAfter runtime.MemStats
			runtime.ReadMemStats(&allocationBefore)
			for range 32 {
				result := make(chan Outcome, 1)
				go func() { result <- r.Capture(context.Background(), "a", candidate(body)) }()
				require.Equal(t, want, await(t, result).Reason, "admission must return while upload is stalled")
				require.Equal(t, before, retained(r), "drops must release exactly their own reservation")
			}
			runtime.ReadMemStats(&allocationAfter)
			if budget {
				require.Less(t, allocationAfter.TotalAlloc-allocationBefore.TotalAlloc, uint64(len(body)), "rejections must not allocate a payload-sized buffer")
			}
			unblock()
			require.NoError(t, r.Shutdown(context.Background()))
			assertReleased(t, r)
		})
	}
}

func TestRecorderInvalidAndOverflow(t *testing.T) {
	r, _, _ := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	for _, mutate := range []func(*Candidate){
		func(c *Candidate) { c.Metadata.PayloadEncoding = "invalid" },
		func(c *Candidate) { c.Metadata.Labels = map[string]string{"a": strings.Repeat("x", MaxLabelValue+1)} },
		func(c *Candidate) { c.Metadata.OriginalProfileID = "invalid\n" },
		func(c *Candidate) { c.Metadata.NativeFormat = "unknown" },
	} {
		c := candidate(nil)
		mutate(&c)
		require.Equal(t, DropInvalid, r.Capture(context.Background(), "a", c).Reason)
		assertReleased(t, r)
	}
	for _, sizes := range [][2]int64{{math.MaxInt64, 1}, {1, math.MaxInt64}, {-1, 0}, {1, -1}} {
		_, ok := r.reservationSize(sizes[0], sizes[1])
		require.False(t, ok)
	}
	// Near MaxInt64, addition would overflow even though both inputs fit int64.
	r.cfg.MaxRetainedBytes = math.MaxInt64
	_, ok := r.reservationSize(math.MaxInt64-itemReservation, 1)
	require.False(t, ok)
}

func TestRecorderTimeoutAndCancellationIgnoringUpload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := recorderTestConfig()
		cfg.UploadTimeout = 2 * time.Second
		cfg.ShutdownDrain = time.Second
		started, proceed := make(chan struct{}), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, _, _ string, body io.Reader) error {
			close(started)
			<-proceed // Models a provider which ignores cancellation.
			_, err := io.Copy(io.Discard, body)
			require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
			return err
		}, nil)
		t.Cleanup(unblock)
		require.True(t, r.Capture(context.Background(), "a", candidate([]byte("owned"))).Enqueued)
		await(t, started)
		before := retained(r)
		time.Sleep(cfg.UploadTimeout)
		synctest.Wait()
		require.Equal(t, before, retained(r))
		require.True(t, r.Capture(context.Background(), "a", candidate([]byte("queued"))).Enqueued)
		require.ErrorIs(t, r.Shutdown(context.Background()), context.DeadlineExceeded)
		require.Equal(t, before, retained(r), "only the queued capture may be released")
		select {
		case <-r.Done():
			t.Fatal("active upload still owns its buffer and borrowed storage")
		default:
		}
		require.Equal(t, 1., testutil.ToFloat64(r.metrics.dropped.WithLabelValues("connect", string(DropShutdown))))
		unblock()
		await(t, r.Done())
		require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "timeout")))
		assertReleased(t, r)
	})
}

func TestRecorderUploadDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, _ string, _ io.Reader) error {
			<-ctx.Done()
			return ctx.Err()
		}, nil)
		require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
		require.NoError(t, r.Shutdown(context.Background()))
		require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("connect", "timeout")))
		assertReleased(t, r)
	})
}

func TestRecorderFixedUploadConcurrency(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.Workers = 2
	cfg.QueueCapacity = 3
	started, proceed := make(chan struct{}, 5), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(proceed) })
	var active, peak atomic.Int64
	r, _, _ := recorderFixture(t, cfg, func(ctx context.Context, tenant, key string, body io.Reader) error {
		n := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); n > previous && !peak.CompareAndSwap(previous, n); previous = peak.Load() {
		}
		started <- struct{}{}
		<-proceed
		return discardUpload(ctx, tenant, key, body)
	}, nil)
	t.Cleanup(unblock)
	for range cfg.Workers {
		require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
		await(t, started)
	}
	for range cfg.QueueCapacity {
		require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	}
	require.Equal(t, DropQueueFull, r.Capture(context.Background(), "a", candidate(nil)).Reason)
	require.Equal(t, int64(cfg.Workers), active.Load())
	unblock()
	require.NoError(t, r.Shutdown(context.Background()))
	require.Equal(t, int64(cfg.Workers), peak.Load())
	assertReleased(t, r)
}
