package profiledump

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

func TestRecorderPolicyChanges(t *testing.T) {
	r, policies, clock := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	lookup := labels.FromStrings("service_name", "checkout")
	c := candidate(nil)
	c.SelectorLabels = lookup
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
			require.Equal(t, tc.reason, r.Capture(context.Background(), "a", c).Reason)
		}
	}
	policies.set("a", Policy{})
	require.Equal(t, DropDisabled, r.Capture(context.Background(), "a", c).Reason)
	policies.set("a", recorderPolicy(t, `{service_name=~"check.*"}`, 1, 10))
	clock.advance(time.Hour)
	require.Equal(t, DropExpired, r.Capture(context.Background(), "a", c).Reason)
}

func TestRecorderIndependentSamplingAndRates(t *testing.T) {
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
	lookup := labels.FromStrings("service_name", "checkout")
	c := candidate(nil)
	c.SelectorLabels = lookup
	for i := range 64 {
		want := DropTenantRate
		if i%2 == 0 {
			want = DropSampled
		} else if i == 1 {
			want = ""
		}
		require.Equal(t, want, r.Capture(context.Background(), "a", c).Reason)
	}
	require.Equal(t, 64, draws)
	// Reload must not refill tokens, even when sampling and selection change.
	policies.set("a", recorderPolicy(t, `{service_name=~"check.*"}`, 1, 5))
	require.Equal(t, DropTenantRate, r.Capture(context.Background(), "a", c).Reason)
	require.Equal(t, 64, draws)
	// Tenant rejection must leave the remaining process token available.
	require.True(t, r.Capture(context.Background(), "b", candidate(nil)).Enqueued)
	policies.set("c", recorderPolicy(t, "{}", 1, 10))
	require.Equal(t, DropProcessRate, r.Capture(context.Background(), "c", candidate(nil)).Reason)
	require.Equal(t, DropTenantRate, r.Capture(context.Background(), "c", candidate(nil)).Reason, "process rejection still consumes the tenant token")
}

func TestRecorderIsolation(t *testing.T) {
	r, policies, _ := recorderFixture(t, recorderTestConfig(), discardUpload, nil)
	p := recorderPolicy(t, `{service_name="checkout"}`, 1, 10)
	policies.set("a", p)
	policies.set("b", p)
	for _, tenant := range []string{"a", "b", "a"} {
		for _, value := range []string{"checkout", "other"} {
			lookup := labels.FromStrings("service_name", value)
			c := candidate(nil)
			c.SelectorLabels = lookup
			for range 2 {
				require.Equal(t, value == "checkout", r.Capture(context.Background(), tenant, c).Enqueued)
			}
		}
	}
	// Reusing a candidate must still evaluate its current labels on every call.
	lookup := labels.FromStrings("service_name", "checkout")
	c := candidate(nil)
	c.SelectorLabels = lookup
	require.True(t, r.Capture(context.Background(), "a", c).Enqueued)
	c.SelectorLabels = labels.FromStrings("service_name", "other")
	require.Equal(t, DropSelector, r.Capture(context.Background(), "a", c).Reason)
}

func TestRecorderSelectorMissDoesNotConsumeTokens(t *testing.T) {
	cfg := recorderTestConfig()
	cfg.TenantBurst, cfg.ProcessBurst = 1, 1
	r, policies, _ := recorderFixture(t, cfg, discardUpload, nil)
	policies.set("a", recorderPolicy(t, `{service_name="checkout"}`, 1, 10))
	c := candidate(nil)
	c.SelectorLabels = labels.FromStrings("service_name", "other")
	for range 64 {
		require.Equal(t, DropSelector, r.Capture(context.Background(), "a", c).Reason)
	}
	c.SelectorLabels = labels.FromStrings("service_name", "checkout")
	require.True(t, r.Capture(context.Background(), "a", c).Enqueued)
}
