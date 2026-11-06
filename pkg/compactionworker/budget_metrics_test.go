package compactionworker

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestBudgetMetrics(t *testing.T) {
	b := newByteBudget(100)
	r := prometheus.NewPedanticRegistry()
	r.MustRegister(newBudgetCollector(b))
	values := func() map[string]float64 {
		families, err := r.Gather()
		require.NoError(t, err)
		result := map[string]float64{}
		for _, f := range families {
			result[f.GetName()] = f.Metric[0].GetGauge().GetValue()
		}
		return result
	}
	release, err := b.acquire(context.Background(), 150, nil)
	require.NoError(t, err)
	require.Equal(t, float64(150), values()["memory_budget_reserved_bytes"])
	ctx, cancel := context.WithCancel(context.Background())
	paused := make(chan struct{})
	done := make(chan error, 1)
	outcomes := make(chan string, 1)
	go func() {
		_, err := b.acquireObserved(ctx, 50, func() { close(paused) }, func(d time.Duration, outcome string) {
			if d < 0 {
				panic("negative wait duration")
			}
			outcomes <- outcome
		})
		done <- err
	}()
	<-paused
	require.Equal(t, float64(50), values()["memory_budget_waiting_bytes"])
	require.Equal(t, float64(1), values()["memory_budget_waiting_jobs"])
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, "canceled", <-outcomes)
	release()
	require.Zero(t, values()["memory_budget_reserved_bytes"])
	require.Zero(t, values()["memory_budget_waiting_jobs"])
	require.Zero(t, values()["memory_budget_waiting_bytes"])
}

func TestBudgetAdmissionObservation(t *testing.T) {
	b := newByteBudget(100)
	release, err := b.acquire(context.Background(), 100, nil)
	require.NoError(t, err)
	paused := make(chan struct{})
	done := make(chan struct{})
	outcomes := make(chan string, 1)
	go func() {
		defer close(done)
		release, err := b.acquireObserved(context.Background(), 50, func() { close(paused) }, func(_ time.Duration, outcome string) { outcomes <- outcome })
		if err == nil {
			release()
		}
	}()
	<-paused
	release()
	<-done
	require.Equal(t, "admitted", <-outcomes)
	require.Empty(t, outcomes)
}

func TestDisabledBudgetMetrics(t *testing.T) {
	b := newByteBudget(0)
	release, err := b.acquireObserved(context.Background(), 100, nil, func(time.Duration, string) { t.Fatal("disabled budget must not observe waits") })
	require.NoError(t, err)
	release()
	require.Zero(t, b.inUse)
	require.Equal(t, 5, testutil.CollectAndCount(newBudgetCollector(b)))
}
