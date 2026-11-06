package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alecthomas/kingpin/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

func TestReplayHTTPResume(t *testing.T) {
	data := []byte("0123456789abcdef")
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("X-Goog-Generation", "123")
		if requests == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			_, _ = w.Write(data[:5])
			return
		}
		if r.Header.Get("Range") != "bytes=5-" {
			t.Errorf("unexpected range %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 5-15/16")
		w.Header().Set("Content-Length", "11")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[5:])
	}))
	defer srv.Close()
	r, err := openReplayInput(context.Background(), srv.URL)
	require.NoError(t, err)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, data, got)
	require.NoError(t, r.Close())
	require.Equal(t, 2, requests)
}

func TestReplayPushCLIFlags(t *testing.T) {
	app := kingpin.New("profilecli", "test replay arguments")
	push := app.Command("replay", "replay").Command("push", "push")
	params := addReplayPushParams(push)
	command, err := app.Parse([]string{"replay", "push", "--input=fixture.replay", "--no-loop", "--url=http://127.0.0.1:4040", "--tenant-id=benchmark"})
	require.NoError(t, err)
	require.Equal(t, "replay push", command)
	require.False(t, params.Loop)
	require.Equal(t, "fixture.replay", params.Input)
}

// fakePusherClient records every push request it receives.
type fakePusherClient struct {
	mu       sync.Mutex
	requests []*pushv1.PushRequest
}

type fakeReplayWaiter struct {
	now   time.Time
	waits []time.Time
}

func (w *fakeReplayWaiter) waitUntil(ctx context.Context, target time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	w.waits = append(w.waits, target)
	if target.After(w.now) {
		w.now = target
	}
	return true
}

func (f *fakePusherClient) Push(_ context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req.Msg)
	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

func (f *fakePusherClient) totalSeries() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		n += len(r.Series)
	}
	return n
}

func (f *fakePusherClient) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakePusherClient) maxBatchSize() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	max := 0
	for _, r := range f.requests {
		if len(r.Series) > max {
			max = len(r.Series)
		}
	}
	return max
}

func testPprofBytes(t *testing.T) []byte {
	t.Helper()
	p := &googlev1.Profile{
		SampleType:  []*googlev1.ValueType{{Type: 1, Unit: 2}},
		PeriodType:  &googlev1.ValueType{Type: 1, Unit: 2},
		StringTable: []string{"", "samples", "count"},
		TimeNanos:   1,
	}
	data, err := pprof.Marshal(p, true)
	require.NoError(t, err)
	return data
}

// TestRunReplayCycle_Batching verifies that many densely-scheduled records
// are coalesced into few push requests (bounded by batch size and batch
// wait), rather than one push request per profile.
func TestRunReplayCycle_Batching(t *testing.T) {
	t.Parallel()

	pprofBytes := testPprofBytes(t)

	const n = 500
	records := make([]replayRecord, n)
	for i := 0; i < n; i++ {
		records[i] = replayRecord{
			Labels: []*typesv1.LabelPair{
				{Name: "service_name", Value: "checkoutservice"},
			},
			// Densely packed: 1ms apart, well within the batch-wait window.
			TimestampNanos: int64(i) * int64(time.Millisecond),
			Pprof:          pprofBytes,
		}
	}

	fake := &fakePusherClient{}
	params := &replayPushParams{
		Speed:     1,
		BatchSize: 50,
		BatchWait: 500 * time.Millisecond,
	}

	cycleStart := time.Unix(0, 0)
	waiter := &fakeReplayWaiter{now: cycleStart}
	pushed, failed, interrupted := runReplayCycleWithWait(context.Background(), fake, records, records[0].TimestampNanos, cycleStart, params, waiter.waitUntil)

	require.False(t, interrupted)
	assert.Equal(t, 0, failed)
	assert.Equal(t, n, pushed)
	assert.Equal(t, n, fake.totalSeries())

	assert.Equal(t, n/params.BatchSize, fake.requestCount())
	assert.Equal(t, params.BatchSize, fake.maxBatchSize())
}

// TestRunReplayCycle_SparseRecordsAreNotOverBatched verifies that records
// scheduled far apart in time (further apart than --batch-wait) are not
// held back waiting for a full batch: each due record is pushed promptly.
func TestRunReplayCycle_SparseRecordsAreNotOverBatched(t *testing.T) {
	t.Parallel()

	pprofBytes := testPprofBytes(t)
	batchWait := 10 * time.Millisecond

	records := []replayRecord{
		{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}}, TimestampNanos: 0, Pprof: pprofBytes},
		{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}}, TimestampNanos: int64(batchWait + time.Nanosecond), Pprof: pprofBytes},
	}

	fake := &fakePusherClient{}
	params := &replayPushParams{
		Speed:     1,
		BatchSize: 50,
		BatchWait: batchWait,
	}

	cycleStart := time.Unix(0, 0)
	waiter := &fakeReplayWaiter{now: cycleStart}
	pushed, failed, interrupted := runReplayCycleWithWait(context.Background(), fake, records, records[0].TimestampNanos, cycleStart, params, waiter.waitUntil)

	require.False(t, interrupted)
	assert.Equal(t, 0, failed)
	assert.Equal(t, 2, pushed)
	// Each record should have been flushed in its own batch since they are
	// scheduled further apart than --batch-wait.
	assert.Equal(t, 2, fake.requestCount())
}

func TestRunReplayCycle_BatchWaitBoundary(t *testing.T) {
	t.Parallel()

	pprofBytes := testPprofBytes(t)
	batchWait := 10 * time.Millisecond
	cycleStart := time.Unix(0, 0)

	tests := []struct {
		name         string
		gap          time.Duration
		requestCount int
	}{
		{name: "before deadline", gap: batchWait - time.Nanosecond, requestCount: 1},
		{name: "at deadline", gap: batchWait, requestCount: 1},
		{name: "after deadline", gap: batchWait + time.Nanosecond, requestCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := []replayRecord{
				{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}}, TimestampNanos: 0, Pprof: pprofBytes},
				{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}}, TimestampNanos: int64(tt.gap), Pprof: pprofBytes},
			}
			fake := &fakePusherClient{}
			waiter := &fakeReplayWaiter{now: cycleStart}
			params := &replayPushParams{Speed: 1, BatchSize: 50, BatchWait: batchWait}

			pushed, failed, interrupted := runReplayCycleWithWait(context.Background(), fake, records, 0, cycleStart, params, waiter.waitUntil)

			assert.False(t, interrupted)
			assert.Zero(t, failed)
			assert.Equal(t, len(records), pushed)
			assert.Equal(t, tt.requestCount, fake.requestCount())
		})
	}
}

func TestRunReplayCycle_ContextCancellation(t *testing.T) {
	t.Parallel()

	pprofBytes := testPprofBytes(t)
	records := []replayRecord{
		{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}}, TimestampNanos: 0, Pprof: pprofBytes},
		{Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}}, TimestampNanos: int64(10 * time.Second), Pprof: pprofBytes},
	}

	fake := &fakePusherClient{}
	params := &replayPushParams{
		Speed:     1,
		BatchSize: 50,
		BatchWait: 500 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately, before the first record's wait

	_, _, interrupted := runReplayCycle(ctx, fake, records, records[0].TimestampNanos, time.Now().Add(20*time.Second), params)
	assert.True(t, interrupted)
}

func TestWaitUntil_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, waitUntil(ctx, time.Now().Add(time.Hour)))
}

func TestBuildSeries_RewritesTimestamp(t *testing.T) {
	t.Parallel()

	pprofBytes := testPprofBytes(t)
	rec := replayRecord{
		Labels:         []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}},
		TimestampNanos: 123,
		Pprof:          pprofBytes,
	}
	target := time.Unix(0, 987654321)

	series, err := buildSeries(rec, target)
	require.NoError(t, err)
	require.Len(t, series.Samples, 1)

	profile, err := pprof.RawFromBytes(series.Samples[0].RawProfile)
	require.NoError(t, err)
	assert.Equal(t, target.UnixNano(), profile.TimeNanos)
	assert.NotEqual(t, rec.TimestampNanos, profile.TimeNanos)
}

func TestReplayFormat_RoundtripWithRealPprof(t *testing.T) {
	t.Parallel()

	pprofBytes := testPprofBytes(t)
	var buf bytes.Buffer
	rw, err := newReplayWriter(&buf, replayHeader{SourceQuery: "{}", Tenants: []string{"t"}})
	require.NoError(t, err)
	require.NoError(t, rw.WriteRecord(replayRecord{
		Labels:         []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}},
		TimestampNanos: 42,
		Pprof:          pprofBytes,
	}))
	require.NoError(t, rw.Flush())

	rr, err := newReplayReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	rec, err := rr.ReadRecord()
	require.NoError(t, err)
	assert.Equal(t, pprofBytes, rec.Pprof)
}

// buildTestDump writes a minimal, valid replay dump file to disk and
// returns its bytes and path.
func buildTestDump(t *testing.T) (path string, data []byte) {
	t.Helper()

	var buf bytes.Buffer
	rw, err := newReplayWriter(&buf, replayHeader{SourceQuery: `{service_name="svc"}`, Tenants: []string{"t"}})
	require.NoError(t, err)
	require.NoError(t, rw.WriteRecord(replayRecord{
		Labels:         []*typesv1.LabelPair{{Name: "service_name", Value: "svc"}},
		TimestampNanos: 1,
		Pprof:          testPprofBytes(t),
	}))
	require.NoError(t, rw.Flush())

	path = filepath.Join(t.TempDir(), "dump.replay")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
	return path, buf.Bytes()
}

func TestOpenReplayReader_LocalFile(t *testing.T) {
	t.Parallel()

	path, _ := buildTestDump(t)
	rr, input, err := openReplayReader(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, input.Close()) })
	assert.Equal(t, `{service_name="svc"}`, rr.Header.SourceQuery)
	_, err = rr.ReadRecord()
	require.NoError(t, err)
}

func TestOpenReplayReader_Zstd(t *testing.T) {
	t.Parallel()

	_, data := buildTestDump(t)
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	require.NoError(t, err)
	_, err = encoder.Write(data)
	require.NoError(t, err)
	require.NoError(t, encoder.Close())
	path := filepath.Join(t.TempDir(), "dump.replay.zst")
	require.NoError(t, os.WriteFile(path, compressed.Bytes(), 0o644))

	rr, input, err := openReplayReader(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, input.Close()) })
	assert.Equal(t, `{service_name="svc"}`, rr.Header.SourceQuery)
	_, err = rr.ReadRecord()
	require.NoError(t, err)
}

func TestOpenReplayReader_HTTPURL(t *testing.T) {
	t.Parallel()

	_, data := buildTestDump(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)

	rr, input, err := openReplayReader(context.Background(), srv.URL+"/dump.replay")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, input.Close()) })
	assert.Equal(t, `{service_name="svc"}`, rr.Header.SourceQuery)
	_, err = rr.ReadRecord()
	require.NoError(t, err)
}

func TestOpenReplayReader_NotFound(t *testing.T) {
	t.Parallel()

	_, _, err := openReplayReader(context.Background(), filepath.Join(t.TempDir(), "does-not-exist.replay"))
	require.Error(t, err)
}
