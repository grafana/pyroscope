package querybackend

import (
	"context"
	"math"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/semaphore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

const (
	// datasetMemoryFactor is the heap a query holds per byte of the dataset
	// it reads: symbol tables decode to a few times their encoded size.
	datasetMemoryFactor = 4
	// memoryBudgetShare is the share of the Go memory limit that dataset
	// queries may hold at once. The rest covers garbage and everything else.
	memoryBudgetShare = 0.6
)

// memoryBudget bounds the memory held by dataset queries running at once.
// A query takes its estimated share before reading and waits while the
// budget is exhausted, so load beyond the budget slows queries down instead
// of exhausting memory.
type memoryBudget struct {
	size  int64
	sem   *semaphore.Weighted
	inUse prometheus.Gauge
	wait  prometheus.Histogram
}

// newMemoryBudget sizes the budget from the Go memory limit. Without a limit
// the budget is unbounded and acquire never waits.
func newMemoryBudget(reg prometheus.Registerer) *memoryBudget {
	b := &memoryBudget{
		inUse: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Subsystem: "query_backend",
			Name:      "memory_budget_in_use_bytes",
			Help:      "Estimated memory held by dataset queries running at once.",
		}),
		wait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Subsystem: "query_backend",
			Name:      "memory_budget_wait_seconds",
			Help:      "Time dataset queries waited for memory budget.",
			Buckets:   prometheus.ExponentialBuckets(0.001, 4, 10),
		}),
	}
	size := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "pyroscope",
		Subsystem: "query_backend",
		Name:      "memory_budget_bytes",
		Help:      "Memory budget for dataset queries running at once; 0 means unbounded.",
	})
	if limit := debug.SetMemoryLimit(-1); limit > 0 && limit != math.MaxInt64 {
		b.size = int64(float64(limit) * memoryBudgetShare)
		b.sem = semaphore.NewWeighted(b.size)
	}
	size.Set(float64(b.size))
	if reg != nil {
		reg.MustRegister(size, b.inUse, b.wait)
	}
	return b
}

// acquire blocks until the dataset fits in the budget or ctx is done. A
// dataset larger than the whole budget waits for the budget to be free and
// then runs alone.
func (b *memoryBudget) acquire(ctx context.Context, ds *metastorev1.Dataset) (release func(), err error) {
	if b.sem == nil {
		return func() {}, nil
	}
	n := min(max(int64(ds.Size)*datasetMemoryFactor, 1), b.size)
	start := time.Now()
	if err = b.sem.Acquire(ctx, n); err != nil {
		return nil, err
	}
	b.wait.Observe(time.Since(start).Seconds())
	b.inUse.Add(float64(n))
	return func() {
		b.inUse.Sub(float64(n))
		b.sem.Release(n)
	}, nil
}
