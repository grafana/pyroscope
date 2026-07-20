package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/runutil"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// progressLogInterval bounds how often "replay progress" is logged while
// pushing a (potentially long-running) cycle, so long replays don't look
// stuck between "starting replay cycle" and "replay cycle complete".
const progressLogInterval = 5 * time.Second

type replayPushParams struct {
	*phlareClient

	Input     string
	Loop      bool
	Speed     float64
	BatchSize int
	BatchWait time.Duration
}

func addReplayPushParams(cmd commander) *replayPushParams {
	params := &replayPushParams{}
	params.phlareClient = addPhlareClient(cmd)

	cmd.Flag("input", "Path to the replay dump file produced by `replay dump`. Accepts a local file path or an http(s) URL.").Short('i').Required().StringVar(&params.Input)
	cmd.Flag("loop", "Continuously repeat the dump, looping every recorded window duration, so the destination cell keeps receiving data that looks like the original recording.").Default("true").BoolVar(&params.Loop)
	cmd.Flag("speed", "Time-scale multiplier for replay speed (2 replays twice as fast, 0.5 half as fast).").Default("1").Float64Var(&params.Speed)
	cmd.Flag("batch-size", "Maximum number of profiles to send in a single push request.").Default("100").IntVar(&params.BatchSize)
	cmd.Flag("batch-wait", "Maximum time to accumulate a batch before flushing it, once the first profile in the batch becomes due.").Default("500ms").DurationVar(&params.BatchWait)
	return params
}

// loadReplayRecords reads the entire dump file into memory, sorted by
// timestamp. Dump files are expected to be bounded in size (a debug/backup
// tool, not a bulk data-transfer mechanism), so loading them fully allows
// the replay loop to schedule pushes precisely without re-reading the file.
//
// input may be a local file path or an http(s) URL.
func loadReplayRecords(ctx context.Context, input string) (replayHeader, []replayRecord, error) {
	r, err := openReplayInput(ctx, input)
	if err != nil {
		return replayHeader{}, nil, err
	}
	defer runutil.CloseWithLogOnErr(logger, r, "failed to close replay dump input")

	rr, err := newReplayReader(r)
	if err != nil {
		return replayHeader{}, nil, err
	}

	var records []replayRecord
	for {
		rec, err := rr.ReadRecord()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return replayHeader{}, nil, err
		}
		records = append(records, rec)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].TimestampNanos < records[j].TimestampNanos
	})

	return rr.Header, records, nil
}

// openReplayInput opens input for reading, transparently supporting either a
// local file path or an http(s) URL (e.g. a signed object storage URL, or a
// dump file served over HTTP). The returned io.ReadCloser must always be
// closed once the reader is no longer needed.
func openReplayInput(ctx context.Context, input string) (io.ReadCloser, error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build request for replay dump file: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch replay dump file: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to fetch replay dump file: unexpected status %s: %s", resp.Status, string(body))
		}
		return resp.Body, nil
	}

	f, err := os.Open(input)
	if err != nil {
		return nil, fmt.Errorf("failed to open replay dump file: %w", err)
	}
	return f, nil
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

	level.Info(logger).Log("msg", "loading replay dump file", "input", params.Input)
	header, records, err := loadReplayRecords(ctx, params.Input)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return errors.New("replay dump file contains no profiles")
	}
	// Only a single tenant is supported for now: push sends everything to one
	// destination tenant (X-Scope-OrgID), so a multi-tenant dump would
	// silently merge tenants on replay.
	if len(header.Tenants) > 1 {
		return fmt.Errorf("replay dump file contains %d tenants (%s); only single-tenant dumps are supported", len(header.Tenants), strings.Join(header.Tenants, ", "))
	}

	minTs := records[0].TimestampNanos
	maxTs := records[len(records)-1].TimestampNanos
	cycleDuration := time.Duration(maxTs - minTs)
	if cycleDuration <= 0 {
		level.Warn(logger).Log("msg", "dump window has no measurable duration (single timestamp); replaying once per second", "profiles", len(records))
		cycleDuration = time.Second
	}

	level.Info(logger).Log("msg", "starting replay push",
		"input", params.Input, "profiles", len(records), "cycle_duration", cycleDuration,
		"source_query", header.SourceQuery, "loop", params.Loop, "speed", params.Speed,
		"batch_size", params.BatchSize, "batch_wait", params.BatchWait, "destination", params.URL)

	pc := params.pusherClient()

	startWall := time.Now()
	for cycle := 0; ; cycle++ {
		if ctx.Err() != nil {
			break
		}
		cycleOffset := time.Duration(float64(cycle) * float64(cycleDuration) / params.Speed)
		cycleStart := startWall.Add(cycleOffset)
		level.Info(logger).Log("msg", "starting replay cycle", "cycle", cycle, "scheduled_start", cycleStart)

		pushed, failed, interrupted := runReplayCycle(ctx, pc, records, minTs, cycleStart, params)

		if interrupted {
			level.Info(logger).Log("msg", "replay interrupted", "cycle", cycle, "pushed", pushed, "failed", failed)
			return nil
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
func runReplayCycle(
	ctx context.Context,
	pc pushv1connect.PusherServiceClient,
	records []replayRecord,
	minTs int64,
	cycleStart time.Time,
	params *replayPushParams,
) (pushed, failed int, interrupted bool) {
	scheduledTarget := func(rec replayRecord) time.Time {
		offset := time.Duration(float64(rec.TimestampNanos-minTs) / params.Speed)
		return cycleStart.Add(offset)
	}

	lastProgressLog := time.Now()
	total := len(records)

	i := 0
	for i < total {
		if ctx.Err() != nil {
			interrupted = true
			break
		}

		first := records[i]
		firstTarget := scheduledTarget(first)
		if !waitUntil(ctx, firstTarget) {
			interrupted = true
			break
		}

		batch := make([]*pushv1.RawProfileSeries, 0, params.BatchSize)
		series, err := buildSeries(first, firstTarget)
		if err != nil {
			failed++
			level.Error(logger).Log("msg", "failed to prepare replayed profile", "err", err)
		} else {
			batch = append(batch, series)
		}
		i++

		deadline := time.Now().Add(params.BatchWait)
		for i < total && len(batch) < params.BatchSize {
			next := records[i]
			nextTarget := scheduledTarget(next)
			if nextTarget.After(deadline) {
				break
			}
			if !waitUntil(ctx, nextTarget) {
				interrupted = true
				break
			}
			if series, err := buildSeries(next, nextTarget); err != nil {
				failed++
				level.Error(logger).Log("msg", "failed to prepare replayed profile", "err", err)
			} else {
				batch = append(batch, series)
			}
			i++
		}

		if len(batch) > 0 {
			if err := pushBatch(ctx, pc, batch); err != nil {
				failed += len(batch)
				level.Error(logger).Log("msg", "failed to push replayed profile batch", "batch_size", len(batch), "err", err)
			} else {
				pushed += len(batch)
				level.Debug(logger).Log("msg", "pushed replayed profile batch", "batch_size", len(batch))
			}
		}

		if interrupted {
			break
		}

		if now := time.Now(); now.Sub(lastProgressLog) >= progressLogInterval {
			level.Info(logger).Log("msg", "replay progress", "pushed", pushed, "failed", failed, "total", total)
			lastProgressLog = now
		}
	}

	return pushed, failed, interrupted
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
