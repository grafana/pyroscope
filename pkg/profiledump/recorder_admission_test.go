package profiledump

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pausedAdmissionLabels delays selection after Capture has read its capture time.
type pausedAdmissionLabels struct {
	clock   *testClock
	entered chan time.Time
	resume  chan struct{}
}

func (l pausedAdmissionLabels) Get(string) string {
	l.entered <- l.clock.now()
	<-l.resume
	return "yes"
}

func TestRecorderAdmissionTimeOrdering(t *testing.T) {
	for _, seriesMode := range []bool{false, true} {
		entry := "direct"
		if seriesMode {
			entry = "series"
		}
		for _, scope := range []string{"tenant", "tenant rate change", "process"} {
			t.Run(entry+"/"+scope, func(t *testing.T) {
				cfg := recorderTestConfig()
				cfg.TenantBurst = 2
				elapsed := time.Second
				limit := 2.0
				wantDrop := DropTenantRate
				if scope == "process" {
					defaults := DefaultRecorderConfig()
					cfg.TenantBurst = defaults.TenantBurst
					cfg.ProcessBurst = defaults.ProcessBurst
					cfg.ProcessCapturesPerSecond = defaults.ProcessCapturesPerSecond
					elapsed = 200 * time.Millisecond
					limit = cfg.ProcessCapturesPerSecond
					wantDrop = DropProcessRate
				}
				r, policies, clock := recorderFixture(t, cfg, discardUpload, nil)
				policy := recorderPolicy(t, `{x="yes"}`, 1, 2)
				policies.set("a", policy)
				tenant := func(kind string, i int) string {
					if scope != "process" {
						return "a"
					}
					id := fmt.Sprintf("%s-%d", kind, i)
					policies.set(id, policy)
					return id
				}
				capture := func(tenant string, lookup LabelLookup) Outcome {
					c := candidate(nil)
					if seriesMode {
						series := r.PrepareSeries(tenant, lookup)
						return series.Capture(context.Background(), c)
					}
					c.SelectorLabels = lookup
					return r.Capture(context.Background(), tenant, c)
				}
				lookup := labels.FromStrings("x", "yes")
				const burst = 2
				admitted := 0
				for i := range burst {
					out := capture(tenant("initial", i), lookup)
					require.Empty(t, out.Reason)
					require.True(t, out.Enqueued)
					admitted++
				}

				const delayedCalls = 5
				resumes := make([]func(), delayedCalls)
				results := make([]chan Outcome, delayedCalls)
				for i := range delayedCalls {
					blocked := pausedAdmissionLabels{clock: clock, entered: make(chan time.Time, 1), resume: make(chan struct{})}
					resumes[i] = sync.OnceFunc(func() { close(blocked.resume) })
					t.Cleanup(resumes[i])
					results[i] = make(chan Outcome, 1)
					id := tenant("delayed", i)
					go func() { results[i] <- capture(id, blocked) }()
					require.Equal(t, recorderNow, await(t, blocked.entered), "delayed selection must start before the clock advances")
				}
				clock.advance(elapsed)
				if scope == "tenant rate change" {
					// Delayed calls retain the old policy while fresh calls update the rate.
					policies.set("a", recorderPolicy(t, `{x="yes"}`, 1, 1))
				}
				for i := range delayedCalls {
					require.Equal(t, recorderNow.Add(elapsed), clock.now())
					fresh := capture(tenant("fresh", i), lookup)
					select {
					case <-results[i]:
						t.Fatal("delayed capture completed before selection was released")
					default:
					}
					// Fresh admission completes before the older capture can reach admission.
					resumes[i]()
					delayed := await(t, results[i])
					for _, result := range []struct {
						name string
						out  Outcome
					}{{"fresh", fresh}, {"delayed", delayed}} {
						want := wantDrop
						if i == 0 {
							want = ""
						}
						assert.Equal(t, want, result.out.Reason, "%s capture %d", result.name, i)
						assert.Equal(t, want == "", result.out.Enqueued, "%s capture %d", result.name, i)
						if result.out.Enqueued {
							admitted++
						}
					}
				}
				// A rate decrease cannot raise the bound set by the original rate.
				bound := burst + int(limit*elapsed.Seconds())
				t.Logf("admitted=%d, bound=%d, elapsed=%s", admitted, bound, elapsed)
				assert.LessOrEqual(t, admitted, bound)
				assert.Equal(t, bound, admitted, "the initial burst and one refill should be available")
			})
		}
	}
}

func TestRecorderAdmissionTimePruningPreservesCaptureTime(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.MaxTenantLimiters = 1
	uploaded := make(chan []byte, 2)
	r, policies, clock := recorderFixture(t, cfg, func(_ context.Context, _, _ string, body io.Reader) error {
		data, err := io.ReadAll(body)
		uploaded <- data
		return err
	}, func(d *Dependencies) {
		// Only admission can prune in this test.
		d.PruneTicks = make(chan time.Time)
	})
	require.True(t, r.Capture(context.Background(), "a", candidate(nil)).Enqueued)
	await(t, uploaded)
	policies.set("b", recorderPolicy(t, `{x="yes"}`, 1, 2))
	blocked := pausedAdmissionLabels{clock: clock, entered: make(chan time.Time, 1), resume: make(chan struct{})}
	resume := sync.OnceFunc(func() { close(blocked.resume) })
	t.Cleanup(resume)
	result := make(chan Outcome, 1)
	go func() {
		c := candidate(nil)
		c.SelectorLabels = blocked
		result <- r.Capture(context.Background(), "b", c)
	}()
	require.Equal(t, recorderNow, await(t, blocked.entered))
	clock.advance(time.Hour)
	resume()
	out := await(t, result)
	require.Empty(t, out.Reason, "admission must prune the tenant that expired during selection")
	require.True(t, out.Enqueued, "the selected capture retains its original policy expiry check")
	m, err := Decode(bytes.NewReader(await(t, uploaded)), io.Discard, cfg.MaxObjectBytes)
	require.NoError(t, err)
	require.Equal(t, recorderNow, m.CapturedAt, "attribution must use capture time, not admission time")
	require.Equal(t, DropExpired, r.Capture(context.Background(), "b", candidate(nil)).Reason)
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.tenants, 1)
	require.Contains(t, r.tenants, "b")
}
