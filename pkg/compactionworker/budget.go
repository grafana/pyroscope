package compactionworker

import (
	"container/list"
	"context"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
)

const defaultMemoryRatio = 0.5

// detectMemoryLimit returns the process memory limit in bytes from GOMEMLIMIT
// (via debug.SetMemoryLimit(-1)) or cgroup memory limits, or 0 if unbounded.
func detectMemoryLimit() int64 {
	var limit int64
	if goLimit := debug.SetMemoryLimit(-1); goLimit > 0 && goLimit < math.MaxInt64 {
		limit = goLimit
	}
	for _, path := range []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
	} {
		if cg := readCgroupMemoryLimit(path); cg > 0 && (limit == 0 || cg < limit) {
			limit = cg
		}
	}
	return limit
}

func readCgroupMemoryLimit(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(data))
	if s == "" || s == "max" {
		return 0
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v == 0 || v >= math.MaxInt64 {
		return 0
	}
	// Linux cgroup v1 reports PAGE_COUNTER_MAX (~math.MaxInt64 rounded to page size) when unlimited.
	if v >= 1<<60 {
		return 0
	}
	return int64(v)
}

func resolveByteCapacity(configuredBytes int64, ratio float64, detectFn func() int64) int64 {
	if configuredBytes > 0 {
		return configuredBytes
	}
	if ratio <= 0 {
		return 0
	}
	limit := detectFn()
	if limit <= 0 {
		return 0
	}
	return int64(float64(limit) * ratio)
}

// estimateJobBytes estimates the memory footprint of compacting blocks.
// Compaction processes one dataset (tenant + service) at a time while holding
// small source objects (<= smallObjectSize) in memory.
func estimateJobBytes(blocks []*metastorev1.BlockMeta, smallObjectSize int) int64 {
	type datasetKey struct {
		tenant  string
		service string
	}
	var (
		smallObjectsBytes uint64
		totalBlockBytes   uint64
		datasetBytes      = make(map[datasetKey]uint64)
		maxDatasetBytes   uint64
	)

	for _, b := range blocks {
		if b == nil {
			continue
		}
		totalBlockBytes += b.Size
		if smallObjectSize > 0 && b.Size <= uint64(smallObjectSize) {
			smallObjectsBytes += b.Size
		}
		for _, ds := range b.Datasets {
			if ds.Name == 0 || ds.Format != uint32(block.DatasetFormat0) {
				continue
			}
			var tenant, service string
			if int(ds.Tenant) < len(b.StringTable) {
				tenant = b.StringTable[ds.Tenant]
			}
			if int(ds.Name) < len(b.StringTable) {
				service = b.StringTable[ds.Name]
			}
			w := block.WeightOf(ds)
			size := w.SymbolsBytes + w.TSDBBytes
			if size == 0 {
				size = ds.Size
			}
			key := datasetKey{tenant: tenant, service: service}
			datasetBytes[key] += size
			if datasetBytes[key] > maxDatasetBytes {
				maxDatasetBytes = datasetBytes[key]
			}
		}
	}

	estimated := smallObjectsBytes + maxDatasetBytes
	if estimated == 0 {
		estimated = totalBlockBytes
	}
	if estimated > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(estimated)
}

type budgetWaiter struct {
	n     int64
	ready chan struct{}
}

// byteBudget coordinates memory-bounded execution of compaction jobs.
// Jobs that exceed the remaining headroom are paused in FIFO order until
// enough capacity is released. A single job larger than the total limit is
// allowed to run once all currently running jobs finish (inUse == 0).
type byteBudget struct {
	mu      sync.Mutex
	limit   int64
	inUse   int64
	waiting int64
	waiters list.List
}

func newByteBudget(limit int64) *byteBudget {
	return &byteBudget{limit: limit}
}

func (b *byteBudget) hasHeadroom() bool {
	if b == nil || b.limit <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.waiters.Len() == 0 && b.inUse < b.limit
}

func (b *byteBudget) canFit(n int64) bool {
	return b.limit <= 0 || b.inUse+n <= b.limit || b.inUse == 0
}

// acquire reserves n bytes from the budget, pausing until capacity is
// available or ctx is canceled. The returned onPause callback (if non-nil)
// is invoked if the caller had to wait for capacity.
func (b *byteBudget) acquire(ctx context.Context, n int64, onPause func()) (release func(), err error) {
	if b == nil || b.limit <= 0 || n <= 0 {
		return func() {}, nil
	}

	b.mu.Lock()
	if b.waiters.Len() == 0 && b.canFit(n) {
		b.inUse += n
		b.mu.Unlock()
		return func() { b.release(n) }, nil
	}

	w := &budgetWaiter{
		n:     n,
		ready: make(chan struct{}),
	}
	elem := b.waiters.PushBack(w)
	b.waiting += n
	b.mu.Unlock()

	if onPause != nil {
		onPause()
	}

	select {
	case <-w.ready:
		return func() { b.release(n) }, nil
	case <-ctx.Done():
		b.mu.Lock()
		select {
		case <-w.ready:
			// Acquired concurrently with cancellation; release immediately.
			b.mu.Unlock()
			b.release(n)
		default:
			b.waiters.Remove(elem)
			b.waiting -= n
			b.notifyLocked()
			b.mu.Unlock()
		}
		return nil, ctx.Err()
	}
}

func (b *byteBudget) release(n int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inUse -= n
	if b.inUse < 0 {
		b.inUse = 0
	}
	b.notifyLocked()
}

func (b *byteBudget) notifyLocked() {
	for b.waiters.Len() > 0 {
		front := b.waiters.Front()
		w := front.Value.(*budgetWaiter)
		if !b.canFit(w.n) {
			break
		}
		b.waiters.Remove(front)
		b.waiting -= w.n
		b.inUse += w.n
		close(w.ready)
	}
}
