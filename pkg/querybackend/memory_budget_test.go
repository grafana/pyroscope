package querybackend

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func newTestBudget(size int64) *memoryBudget {
	return &memoryBudget{
		size:  size,
		sem:   semaphore.NewWeighted(size),
		inUse: prometheus.NewGauge(prometheus.GaugeOpts{Name: "in_use"}),
		wait:  prometheus.NewHistogram(prometheus.HistogramOpts{Name: "wait"}),
	}
}

func Test_memoryBudget_waits_when_exhausted(t *testing.T) {
	b := newTestBudget(100 * datasetMemoryFactor)
	ctx := context.Background()

	r1, err := b.acquire(ctx, &metastorev1.Dataset{Size: 60})
	require.NoError(t, err)

	acquired := make(chan func())
	go func() {
		r, err := b.acquire(ctx, &metastorev1.Dataset{Size: 60})
		require.NoError(t, err)
		acquired <- r
	}()
	select {
	case <-acquired:
		t.Fatal("acquired over budget")
	case <-time.After(50 * time.Millisecond):
	}
	r1()
	select {
	case r2 := <-acquired:
		r2()
	case <-time.After(time.Second):
		t.Fatal("not acquired after release")
	}
}

func Test_memoryBudget_oversized_dataset_runs_alone(t *testing.T) {
	b := newTestBudget(100)
	release, err := b.acquire(context.Background(), &metastorev1.Dataset{Size: 1 << 30})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = b.acquire(ctx, &metastorev1.Dataset{Size: 1})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	release()
}

func Test_memoryBudget_unbounded_without_limit(t *testing.T) {
	b := &memoryBudget{}
	release, err := b.acquire(context.Background(), &metastorev1.Dataset{Size: 1 << 40})
	require.NoError(t, err)
	release()
}
