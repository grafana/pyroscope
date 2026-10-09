package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// progressLogInterval bounds how often "replay progress" is logged while
// pushing a (potentially long-running) cycle, so long replays don't look
// stuck between "starting replay cycle" and "replay cycle complete".
const progressLogInterval = 5 * time.Second

// replayPushPool manages a fixed number of goroutines that drain a batch
// channel and call pushBatch concurrently. The scheduler goroutine submits
// batches via submit(); the channel acts as a semaphore so the scheduler
// naturally stalls when all workers are busy. Call wait() once all batches
// have been submitted: it closes the channel, waits for every worker to
// finish, and returns the aggregate pushed/failed counts.
//
// Context cancellation is handled cleanly: submit() selects on ctx.Done() so
// the scheduler is never stuck, and workers that fail solely because the
// context was cancelled do not increment the failed counter.
type replayPushPool struct {
	ctx    context.Context
	ch     chan []*pushv1.RawProfileSeries
	wg     sync.WaitGroup
	pushed atomic.Int64
	failed atomic.Int64
}

func newReplayPushPool(ctx context.Context, pc pushv1connect.PusherServiceClient, workers int) *replayPushPool {
	p := &replayPushPool{
		ctx: ctx,
		ch:  make(chan []*pushv1.RawProfileSeries, workers),
	}
	for range workers {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for batch := range p.ch {
				if err := pushBatch(ctx, pc, batch); err != nil {
					// Don't count failures that are purely due to context
					// cancellation; those are expected during a clean shutdown.
					if ctx.Err() != nil {
						return
					}
					p.failed.Add(int64(len(batch)))
					level.Error(logger).Log("msg", "failed to push replayed profile batch", "batch_size", len(batch), "err", err)
				} else {
					p.pushed.Add(int64(len(batch)))
					level.Debug(logger).Log("msg", "pushed replayed profile batch", "batch_size", len(batch))
				}
			}
		}()
	}
	return p
}

// submit enqueues a batch for a worker. It returns false (without blocking)
// if the context is cancelled, so the scheduler is never stuck when shutting
// down even if all workers are temporarily busy.
func (p *replayPushPool) submit(batch []*pushv1.RawProfileSeries) bool {
	select {
	case p.ch <- batch:
		return true
	case <-p.ctx.Done():
		return false
	}
}

func (p *replayPushPool) wait() (pushed, failed int) {
	close(p.ch)
	p.wg.Wait()
	return int(p.pushed.Load()), int(p.failed.Load())
}

// effectiveWorkers returns n, clamped to a minimum of 1. A configured value
// of 0 (the zero value, used in tests) is treated as 1 to keep tests
// deterministic without requiring callers to set an explicit default.
func effectiveWorkers(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

type replayPushParams struct {
	*phlareClient

	Input          string
	Loop           bool
	Speed          float64
	BatchSize      int
	BatchWait      time.Duration
	Workers        int
	ExpectedSHA256 string
}

func addReplayPushParams(cmd commander) *replayPushParams {
	params := &replayPushParams{}
	params.phlareClient = addPhlareClient(cmd)

	cmd.Flag("input", "Path to the replay dump file produced by `replay dump`. Accepts a local file path or an http(s) URL.").Short('i').Required().StringVar(&params.Input)
	cmd.Flag("loop", "Continuously repeat the dump, looping every recorded window duration, so the destination cell keeps receiving data that looks like the original recording.").Default("true").BoolVar(&params.Loop)
	cmd.Flag("speed", "Time-scale multiplier for replay speed (2 replays twice as fast, 0.5 half as fast).").Default("1").Float64Var(&params.Speed)
	cmd.Flag("batch-size", "Maximum number of profiles to send in a single push request.").Default("100").IntVar(&params.BatchSize)
	cmd.Flag("batch-wait", "Maximum time to accumulate a batch before flushing it, once the first profile in the batch becomes due.").Default("500ms").DurationVar(&params.BatchWait)
	cmd.Flag("workers", "Number of concurrent push workers. Each worker sends one batch at a time; increasing this hides network round-trip latency.").Default("16").IntVar(&params.Workers)
	cmd.Flag("sha256", "Expected SHA-256 hex digest of the raw (compressed) input file. If set, the digest is verified after each read pass; an error is returned on mismatch. When omitted the computed digest is still logged.").StringVar(&params.ExpectedSHA256)
	return params
}

// rawHashReadCloser wraps a decompressed replay reader and tracks the SHA-256
// of the underlying raw (possibly compressed) bytes as they are consumed.
// Call HexSum() after the reader is fully drained and closed to obtain the
// digest.
type rawHashReadCloser struct {
	io.ReadCloser
	hexSum string
}

// HexSum returns the hex-encoded SHA-256 digest of the raw bytes consumed.
// It is only valid after the reader has been fully read and closed.
func (r *rawHashReadCloser) HexSum() string { return r.hexSum }

// openReplayInput opens a local file or HTTP(S) URL and detects an optional
// outer Zstandard stream from its first four bytes. The returned reader always
// yields the replay format itself, not its optional transport compression.
// The raw (pre-decompression) bytes are hashed with SHA-256 as they are read;
// the digest is available via HexSum() after the reader is closed.
func openReplayInput(ctx context.Context, input string) (*rawHashReadCloser, error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build request for replay dump file: %w", err)
		}
		req.Header.Set("Accept-Encoding", "identity")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch replay dump file: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to fetch replay dump file: unexpected status %s: %s", resp.Status, string(body))
		}
		if resp.ContentLength < 0 {
			_ = resp.Body.Close()
			return nil, errors.New("replay HTTP input requires a known content length")
		}
		return newRawHashReadCloser(&resumableReplayBody{ctx: ctx, url: input, body: resp.Body, size: resp.ContentLength, generation: resp.Header.Get("X-Goog-Generation")}), nil
	}

	f, err := os.Open(input)
	if err != nil {
		return nil, fmt.Errorf("failed to open replay dump file: %w", err)
	}
	return newRawHashReadCloser(f), nil
}

// resumableReplayBody retries only source reads, below hashing and decompression.
// Bytes returned to the caller are never fetched again.
type resumableReplayBody struct {
	ctx        context.Context
	url        string
	body       io.ReadCloser
	size       int64
	offset     int64
	generation string
	retries    int
}

func (r *resumableReplayBody) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.offset == r.size {
		return 0, io.EOF
	}
	for {
		n, err := r.body.Read(p)
		r.offset += int64(n)
		if n > 0 {
			r.retries = 0
			if r.offset > r.size {
				return n, errors.New("replay HTTP body exceeds declared size")
			}
			return n, nil
		}
		if err == nil {
			return 0, nil
		}
		if r.offset == r.size {
			return 0, io.EOF
		}
		if r.retries >= 5 {
			return 0, fmt.Errorf("replay HTTP stream interrupted at byte %d: %w", r.offset, err)
		}
		r.retries++
		_ = r.body.Close()
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		case <-time.After(time.Duration(r.retries) * time.Second):
		}
		req, e := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
		if e != nil {
			return 0, e
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", r.offset))
		req.Header.Set("Accept-Encoding", "identity")
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			continue
		}
		want := fmt.Sprintf("bytes %d-%d/%d", r.offset, r.size-1, r.size)
		if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != want || resp.ContentLength != r.size-r.offset || (r.generation != "" && resp.Header.Get("X-Goog-Generation") != r.generation) {
			_ = resp.Body.Close()
			return 0, fmt.Errorf("replay HTTP resume at byte %d: unexpected response %s (Content-Range %q)", r.offset, resp.Status, resp.Header.Get("Content-Range"))
		}
		r.body = resp.Body
	}
}

func (r *resumableReplayBody) Close() error { return r.body.Close() }

// newRawHashReadCloser wraps raw in a SHA-256 tee, then layers optional zstd
// decompression on top. The hasher sees every raw byte read from the source.
func newRawHashReadCloser(raw io.ReadCloser) *rawHashReadCloser {
	h := sha256.New()
	tee := io.TeeReader(raw, h)                 // hash raw bytes transparently
	dec := replayInputReader(io.NopCloser(tee)) // zstd-detect on the tee
	rc := &rawHashReadCloser{}
	rc.ReadCloser = replayInputReadCloser{
		Reader: dec,
		close: func() error {
			err := dec.Close()
			rc.hexSum = hex.EncodeToString(h.Sum(nil))
			rawErr := raw.Close()
			if err == nil {
				err = rawErr
			}
			return err
		},
	}
	return rc
}

var replayZstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}

type replayInputReadCloser struct {
	io.Reader
	close func() error
}

func (r replayInputReadCloser) Close() error { return r.close() }

func replayInputReader(r io.ReadCloser) io.ReadCloser {
	br := bufio.NewReader(r)
	magic, err := br.Peek(len(replayZstdMagic))
	if err == nil && string(magic) == string(replayZstdMagic) {
		decoder, err := zstd.NewReader(br)
		if err == nil {
			return replayInputReadCloser{Reader: decoder, close: func() error {
				decoder.Close()
				return r.Close()
			}}
		}
	}
	return replayInputReadCloser{Reader: br, close: r.Close}
}

func openReplayReader(ctx context.Context, input string) (*replayReader, *rawHashReadCloser, error) {
	r, err := openReplayInput(ctx, input)
	if err != nil {
		return nil, nil, err
	}
	rr, err := newReplayReader(r)
	if err != nil {
		_ = r.Close()
		return nil, nil, err
	}
	return rr, r, nil
}

func replayPush(ctx context.Context, params *replayPushParams) error {
	if params.Speed <= 0 {
		return errors.New("--speed must be greater than 0")
	}
	if params.BatchSize < 1 {
		return errors.New("--batch-size must be at least 1")
	}
	if params.BatchWait < 0 {
		return errors.New("--batch-wait must not be negative")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if params.ExpectedSHA256 != "" {
		norm := strings.ToLower(strings.TrimSpace(params.ExpectedSHA256))
		if len(norm) != 64 {
			return fmt.Errorf("--sha256: expected a 64-character hex string, got %d characters", len(norm))
		}
		if _, err := hex.DecodeString(norm); err != nil {
			return fmt.Errorf("--sha256: not valid hex: %w", err)
		}
		params.ExpectedSHA256 = norm
	}

	level.Info(logger).Log("msg", "opening replay dump file", "input", params.Input)
	rr, input, err := openReplayReader(ctx, params.Input)
	if err != nil {
		return err
	}
	// Keep this reader for the first cycle; probing must not consume the
	// entire input before streaming can begin.
	defer func() {
		if input != nil {
			_ = input.Close()
		}
	}()
	header := rr.Header
	if len(header.Tenants) > 1 {
		return fmt.Errorf("replay dump file contains %d tenants (%s); only single-tenant dumps are supported", len(header.Tenants), strings.Join(header.Tenants, ", "))
	}

	cycleDuration := time.Duration(header.To-header.From) * time.Millisecond
	if cycleDuration <= 0 {
		level.Warn(logger).Log("msg", "dump window has no measurable duration; replaying once per second")
		cycleDuration = time.Second
	}
	level.Info(logger).Log("msg", "starting replay push",
		"input", params.Input, "cycle_duration", cycleDuration, "source_query", header.SourceQuery,
		"loop", params.Loop, "speed", params.Speed, "batch_size", params.BatchSize,
		"batch_wait", params.BatchWait, "destination", params.URL)

	pc := params.pusherClient()
	startWall := time.Now()
	for cycle := 0; ; cycle++ {
		if ctx.Err() != nil {
			break
		}
		cycleOffset := time.Duration(float64(cycle) * float64(cycleDuration) / params.Speed)
		cycleStart := startWall.Add(cycleOffset)
		level.Info(logger).Log("msg", "starting replay cycle", "cycle", cycle, "scheduled_start", cycleStart)

		if cycle > 0 {
			rr, input, err = openReplayReader(ctx, params.Input)
			if err != nil {
				return err
			}
		}
		var pushed, failed int
		var interrupted bool
		var runErr error
		if header.Version >= replayFormatVersion {
			// v2+: records are guaranteed timestamp-ordered; stream them.
			first, readErr := rr.ReadRecord()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return errors.New("replay dump file contains no profiles")
				}
				return fmt.Errorf("failed to read first replay record: %w", readErr)
			}
			pushed, failed, interrupted, runErr = runReplayReaderCycle(ctx, pc, rr, first, cycleStart, params)
		} else {
			// v1: ordering is not guaranteed; read all records and sort before replaying.
			level.Info(logger).Log("msg", "v1 replay dump: buffering and sorting all records by timestamp", "cycle", cycle)
			records, minTs, loadErr := loadAndSortRecords(rr)
			if loadErr != nil {
				return fmt.Errorf("failed to load replay records: %w", loadErr)
			}
			if len(records) == 0 {
				return errors.New("replay dump file contains no profiles")
			}
			pushed, failed, interrupted = runReplayCycle(ctx, pc, records, minTs, cycleStart, params)
		}
		closeErr := input.Close()
		digest := input.HexSum()
		input = nil
		if runErr != nil {
			return runErr
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close replay dump input: %w", closeErr)
		}
		if interrupted || ctx.Err() != nil {
			level.Info(logger).Log("msg", "replay interrupted", "cycle", cycle, "pushed", pushed, "failed", failed)
			return nil
		}
		if shaErr := checkReplaySHA256(params.Input, digest, params.ExpectedSHA256); shaErr != nil {
			return shaErr
		}
		level.Info(logger).Log("msg", "replay cycle complete", "cycle", cycle, "pushed", pushed, "failed", failed)
		if !params.Loop {
			break
		}
	}
	return nil
}

// runReplayCycle pushes every record once, grouping consecutive due records
// into batches of up to params.BatchSize, flushed as soon as either the
// batch is full or params.BatchWait has elapsed since the first profile in
// the batch became due. Batching a whole cycle's worth of profiles into far
// fewer push requests keeps up with schedules that would otherwise require
// hundreds of individual round-trips per second.
// runReplayReaderCycle streams one timestamp-ordered dump cycle. It retains
// only the current push batch and one look-ahead record in memory.
func runReplayReaderCycle(
	ctx context.Context,
	pc pushv1connect.PusherServiceClient,
	rr *replayReader,
	first replayRecord,
	cycleStart time.Time,
	params *replayPushParams,
) (pushed, failed int, interrupted bool, err error) {
	pool := newReplayPushPool(ctx, pc, effectiveWorkers(params.Workers))
	defer func() {
		p, f := pool.wait()
		pushed += p
		failed += f
	}()

	scheduledTarget := func(rec replayRecord) time.Time {
		return cycleStart.Add(time.Duration(float64(rec.TimestampNanos-first.TimestampNanos) / params.Speed))
	}
	current := first
	var buildFailed int
	lastProgressLog := time.Now()
	for {
		if ctx.Err() != nil {
			interrupted = true
			return
		}
		firstTarget := scheduledTarget(current)
		if !waitUntil(ctx, firstTarget) {
			interrupted = true
			return
		}
		batch := make([]*pushv1.RawProfileSeries, 0, params.BatchSize)
		for {
			series, buildErr := buildSeries(current, scheduledTarget(current))
			if buildErr != nil {
				buildFailed++
				level.Error(logger).Log("msg", "failed to prepare replayed profile", "err", buildErr)
			} else {
				batch = append(batch, series)
			}

			next, readErr := rr.ReadRecord()
			if errors.Is(readErr, io.EOF) {
				if len(batch) > 0 && !pool.submit(batch) {
					interrupted = true
				}
				failed += buildFailed
				return
			}
			if readErr != nil {
				failed += buildFailed
				err = fmt.Errorf("failed to read replay record: %w", readErr)
				return
			}
			if len(batch) == params.BatchSize || scheduledTarget(next).After(firstTarget.Add(params.BatchWait)) {
				if len(batch) > 0 && !pool.submit(batch) {
					interrupted = true
					failed += buildFailed
					return
				}
				current = next
				break
			}
			if !waitUntil(ctx, scheduledTarget(next)) {
				interrupted = true
				failed += buildFailed
				return
			}
			current = next
		}
		if time.Since(lastProgressLog) >= progressLogInterval {
			level.Info(logger).Log("msg", "replay progress", "pushed", pool.pushed.Load(), "failed", pool.failed.Load()+int64(buildFailed))
			lastProgressLog = time.Now()
		}
	}
}

// loadAndSortRecords reads all records from rr into memory and sorts them by
// ascending timestamp. It is used for v1 dump files where ordering is not
// guaranteed. Returns the sorted slice and the minimum timestamp.
func loadAndSortRecords(rr *replayReader) ([]replayRecord, int64, error) {
	var records []replayRecord
	for {
		rec, err := rr.ReadRecord()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].TimestampNanos < records[j].TimestampNanos
	})
	var minTs int64
	if len(records) > 0 {
		minTs = records[0].TimestampNanos
	}
	return records, minTs, nil
}

func runReplayCycle(
	ctx context.Context,
	pc pushv1connect.PusherServiceClient,
	records []replayRecord,
	minTs int64,
	cycleStart time.Time,
	params *replayPushParams,
) (pushed, failed int, interrupted bool) {
	return runReplayCycleWithWait(ctx, pc, records, minTs, cycleStart, params, waitUntil)
}

type replayWaitFunc func(context.Context, time.Time) bool

func runReplayCycleWithWait(
	ctx context.Context,
	pc pushv1connect.PusherServiceClient,
	records []replayRecord,
	minTs int64,
	cycleStart time.Time,
	params *replayPushParams,
	wait replayWaitFunc,
) (pushed, failed int, interrupted bool) {
	pool := newReplayPushPool(ctx, pc, effectiveWorkers(params.Workers))
	defer func() {
		p, f := pool.wait()
		pushed += p
		failed += f
	}()

	scheduledTarget := func(rec replayRecord) time.Time {
		offset := time.Duration(float64(rec.TimestampNanos-minTs) / params.Speed)
		return cycleStart.Add(offset)
	}

	lastProgressLog := time.Now()
	total := len(records)
	var buildFailed int

	i := 0
	for i < total {
		if ctx.Err() != nil {
			interrupted = true
			break
		}

		first := records[i]
		firstTarget := scheduledTarget(first)
		if !wait(ctx, firstTarget) {
			interrupted = true
			break
		}

		batch := make([]*pushv1.RawProfileSeries, 0, params.BatchSize)
		series, err := buildSeries(first, firstTarget)
		if err != nil {
			buildFailed++
			level.Error(logger).Log("msg", "failed to prepare replayed profile", "err", err)
		} else {
			batch = append(batch, series)
		}
		i++

		deadline := firstTarget.Add(params.BatchWait)
		for i < total && len(batch) < params.BatchSize {
			next := records[i]
			nextTarget := scheduledTarget(next)
			if nextTarget.After(deadline) {
				break
			}
			if !wait(ctx, nextTarget) {
				interrupted = true
				break
			}
			if series, err := buildSeries(next, nextTarget); err != nil {
				buildFailed++
				level.Error(logger).Log("msg", "failed to prepare replayed profile", "err", err)
			} else {
				batch = append(batch, series)
			}
			i++
		}

		if len(batch) > 0 && !pool.submit(batch) {
			interrupted = true
		}

		if interrupted {
			break
		}

		if now := time.Now(); now.Sub(lastProgressLog) >= progressLogInterval {
			level.Info(logger).Log("msg", "replay progress", "pushed", pool.pushed.Load(), "failed", pool.failed.Load()+int64(buildFailed), "total", total)
			lastProgressLog = now
		}
	}
	failed += buildFailed
	return
}

// waitUntil blocks until target, or returns false immediately if ctx is
// cancelled first.
func waitUntil(ctx context.Context, target time.Time) bool {
	d := time.Until(target)
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// checkReplaySHA256 logs the computed SHA-256 digest of the raw input bytes
// and, when expected is non-empty, returns an error if the digests do not match.
func checkReplaySHA256(input, computed, expected string) error {
	if expected == "" {
		level.Info(logger).Log("msg", "replay input sha256", "input", input, "sha256", computed)
		return nil
	}
	if computed != expected {
		return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", input, expected, computed)
	}
	level.Info(logger).Log("msg", "replay input sha256 verified", "input", input, "sha256", computed)
	return nil
}

// buildSeries reconstructs the pprof profile with its timestamp rewritten to
// target (the scheduled wall-clock replay time), ready to be included in a
// push request.
func buildSeries(rec replayRecord, target time.Time) (*pushv1.RawProfileSeries, error) {
	profile, err := pprof.RawFromBytes(rec.Pprof)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pprof: %w", err)
	}
	profile.TimeNanos = target.UnixNano()
	data, err := pprof.Marshal(profile.Profile, true)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pprof: %w", err)
	}

	return &pushv1.RawProfileSeries{
		Labels: rec.Labels,
		Samples: []*pushv1.RawSample{{
			ID:         uuid.New().String(),
			RawProfile: data,
		}},
	}, nil
}

func pushBatch(ctx context.Context, pc pushv1connect.PusherServiceClient, batch []*pushv1.RawProfileSeries) error {
	_, err := pc.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
		Series: batch,
	}))
	return err
}
