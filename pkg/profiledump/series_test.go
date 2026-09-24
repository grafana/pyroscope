package profiledump

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

type countingLabelLookup struct {
	value string
	calls int
}

func (l *countingLabelLookup) Get(string) string {
	l.calls++
	return l.value
}

func TestSeriesCaptureReusesSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  string
		reason DropReason
	}{
		{name: "matching", value: strings.Repeat("x", 1<<20)},
		{name: "tenant rate", value: strings.Repeat("x", 1<<20), reason: DropTenantRate},
		{name: "process rate", value: strings.Repeat("x", 1<<20), reason: DropProcessRate},
		{name: "nonmatching", value: strings.Repeat("y", 1<<20), reason: DropSelector},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := recorderTestConfig()
			if tc.reason == DropTenantRate {
				cfg.TenantBurst = 1
			}
			if tc.reason == DropProcessRate {
				cfg.ProcessBurst = 1
			}
			r, policies, _ := recorderFixture(t, cfg, discardUpload, nil)
			policies.set("a", recorderPolicy(t, `{oversized=~"(xy?)+"}`, 1, 10))
			lookup := &countingLabelLookup{value: tc.value}
			series := r.PrepareSeries("a", lookup)
			for i := range 64 {
				out := series.Capture(context.Background(), candidate(nil))
				want := tc.reason
				if i == 0 && (want == DropTenantRate || want == DropProcessRate) {
					want = ""
				}
				require.Equal(t, want, out.Reason)
				require.Equal(t, want == "", out.Enqueued)
			}
			require.Equal(t, 1, lookup.calls)
			if tc.reason != "" {
				want := 64.
				if tc.reason == DropTenantRate || tc.reason == DropProcessRate {
					want--
				}
				require.Equal(t, want, testutil.ToFloat64(r.metrics.dropped.WithLabelValues("connect", string(tc.reason))))
			}
		})
	}
}

func TestSeriesCapturePolicyChanges(t *testing.T) {
	r, policies, clock := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	lookup := &countingLabelLookup{value: "checkout"}
	series := r.PrepareSeries("a", lookup)
	for _, tc := range []struct {
		selector string
		reason   DropReason
	}{
		{selector: `{service_name="checkout"}`},
		{selector: `{service_name="other"}`, reason: DropSelector},
		{selector: `{service_name=~"check.*"}`},
	} {
		policies.set("a", recorderPolicy(t, tc.selector, 1, 10))
		for range 2 {
			require.Equal(t, tc.reason, series.Capture(context.Background(), candidate(nil)).Reason)
		}
	}
	require.Equal(t, 3, lookup.calls, "each changed policy must reevaluate selection")
	policies.set("a", Policy{})
	require.Equal(t, DropDisabled, series.Capture(context.Background(), candidate(nil)).Reason)
	policies.set("a", recorderPolicy(t, `{service_name=~"check.*"}`, 1, 10))
	clock.advance(time.Hour)
	require.Equal(t, DropExpired, series.Capture(context.Background(), candidate(nil)).Reason)
	require.Equal(t, 3, lookup.calls, "inactive policies stop before matching")
}

func TestSeriesCaptureIndependentSamplingAndRates(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.TenantBurst, cfg.ProcessBurst = 1, 2
	draws := 0
	r, policies, _ := recorderFixture(t, cfg, discardUpload, func(d *Dependencies) {
		d.Random = func() float64 {
			draws++
			if draws%2 == 1 {
				return .9
			}
			return .1
		}
	})
	policies.set("a", recorderPolicy(t, `{service_name="checkout"}`, .5, 10))
	lookup := &countingLabelLookup{value: "checkout"}
	series := r.PrepareSeries("a", lookup)
	for i := range 64 {
		want := DropTenantRate
		if i%2 == 0 {
			want = DropSampled
		} else if i == 1 {
			want = ""
		}
		require.Equal(t, want, series.Capture(context.Background(), candidate(nil)).Reason)
	}
	require.Equal(t, 64, draws)
	require.Equal(t, 1, lookup.calls)
	// Reload must not refill tokens, even when sampling and selection change.
	policies.set("a", recorderPolicy(t, `{service_name=~"check.*"}`, 1, 5))
	require.Equal(t, DropTenantRate, series.Capture(context.Background(), candidate(nil)).Reason)
	require.Equal(t, 2, lookup.calls)
	require.Equal(t, 64, draws)
	// Tenant rejection must leave the remaining process token available.
	other := r.PrepareSeries("b", nil)
	require.True(t, other.Capture(context.Background(), candidate(nil)).Enqueued)
	policies.set("c", recorderPolicy(t, "{}", 1, 10))
	third := r.PrepareSeries("c", nil)
	require.Equal(t, DropProcessRate, third.Capture(context.Background(), candidate(nil)).Reason)
	require.Equal(t, DropTenantRate, third.Capture(context.Background(), candidate(nil)).Reason, "process rejection still consumes the tenant token")
}

func TestSeriesCaptureIsolation(t *testing.T) {
	r, policies, _ := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	p := recorderPolicy(t, `{service_name="checkout"}`, 1, 10)
	policies.set("a", p)
	policies.set("b", p)
	for _, tenant := range []string{"a", "b", "a"} {
		for _, value := range []string{"checkout", "other"} {
			lookup := &countingLabelLookup{value: value}
			series := r.PrepareSeries(tenant, lookup)
			for range 2 {
				require.Equal(t, value == "checkout", series.Capture(context.Background(), candidate(nil)).Enqueued)
			}
			require.Equal(t, 1, lookup.calls, "each series gets its own selection result")
		}
	}
	// Direct callers still evaluate their own labels on every call.
	lookup := &countingLabelLookup{value: "checkout"}
	c := candidate(nil)
	c.SelectorLabels = lookup
	require.True(t, r.Capture(context.Background(), "a", c).Enqueued)
	lookup.value = "other"
	require.Equal(t, DropSelector, r.Capture(context.Background(), "a", c).Reason)
	require.Equal(t, 2, lookup.calls)
}

func TestSeriesCaptureSelectorMissDoesNotConsumeTokens(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.TenantBurst, cfg.ProcessBurst = 1, 1
	r, policies, _ := recorderFixture(t, cfg, discardUpload, nil)
	policies.set("a", recorderPolicy(t, `{service_name="checkout"}`, 1, 10))
	miss := r.PrepareSeries("a", &countingLabelLookup{value: "other"})
	for range 64 {
		require.Equal(t, DropSelector, miss.Capture(context.Background(), candidate(nil)).Reason)
	}
	hit := r.PrepareSeries("a", &countingLabelLookup{value: "checkout"})
	require.True(t, hit.Capture(context.Background(), candidate(nil)).Enqueued)
}
