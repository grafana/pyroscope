package adaptive_placement

import (
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/ewma"
	"github.com/grafana/pyroscope/pkg/iter"
)

// DistributionStats is a helper struct that tracks the data rate of each
// dataset within a certain time window. EWMA aggregation function is used
// to calculate the instantaneous rate of the dataset, the time window is
// half-life of the EWMA function.
//
// DistributionStats is safe for concurrent use.
type DistributionStats struct {
	mu       sync.Mutex
	counters map[counterKey]*ewma.Rate
	window   time.Duration
}

func NewDistributionStats(window time.Duration) *DistributionStats {
	return &DistributionStats{
		counters: make(map[counterKey]*ewma.Rate),
		window:   window,
	}
}

type Sample struct {
	TenantID    string
	DatasetName string
	ShardOwner  string
	ShardID     uint32
	Size        uint64
}

func (d *DistributionStats) RecordStats(samples iter.Iterator[Sample]) {
	d.recordStats(time.Now().UnixNano(), samples)
}

func (d *DistributionStats) Build() *adaptive_placementpb.DistributionStats {
	return d.build(time.Now().UnixNano())
}

func (d *DistributionStats) Expire(before time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, v := range d.counters {
		if v.LastUpdate().Before(before) {
			delete(d.counters, k)
		}
	}
}

func (d *DistributionStats) recordStats(now int64, samples iter.Iterator[Sample]) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for samples.Next() {
		s := samples.At()
		// TODO(kolesnikovae): intern strings with unique (go 1.23)
		c := d.counter(counterKey{
			tenant:  s.TenantID,
			dataset: s.DatasetName,
			shard: shard{
				owner: s.ShardOwner,
				id:    s.ShardID,
			},
		})
		c.UpdateAt(float64(s.Size), now)
	}
}

func (d *DistributionStats) counter(k counterKey) *ewma.Rate {
	c, ok := d.counters[k]
	if !ok {
		c = ewma.NewHalfLife(d.window)
		d.counters[k] = c
	}
	return c
}

type counterKey struct {
	tenant  string
	dataset string
	shard   shard
}

func (k counterKey) compare(x counterKey) int {
	if c := strings.Compare(k.tenant, x.tenant); c != 0 {
		return c
	}
	if c := strings.Compare(k.dataset, x.dataset); c != 0 {
		return c
	}
	if k.shard.id < x.shard.id {
		return -1
	}
	if k.shard.id > x.shard.id {
		return 1
	}
	return strings.Compare(k.shard.owner, x.shard.owner)
}

type shard struct {
	owner string
	id    uint32
}

func (d *DistributionStats) build(now int64) *adaptive_placementpb.DistributionStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	tenants := make(map[string]int)
	datasets := make(map[string]int)
	shards := make(map[shard]int)

	// Although, not strictly required, we iterate over the keys
	// in a deterministic order to make the output deterministic.
	keys := make([]counterKey, 0, len(d.counters))
	for k := range d.counters {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b counterKey) int {
		return a.compare(b)
	})

	stats := &adaptive_placementpb.DistributionStats{CreatedAt: now}
	for _, k := range keys {
		c := d.counters[k]
		// Skip dataset-wide counters.
		if k.shard.id == 0 {
			continue
		}

		ti, ok := tenants[k.tenant]
		if !ok {
			ti = len(stats.Tenants)
			tenants[k.tenant] = ti
			stats.Tenants = append(stats.Tenants, &adaptive_placementpb.TenantStats{
				TenantId: k.tenant,
			})
		}

		di, ok := datasets[k.dataset]
		if !ok {
			di = len(stats.Datasets)
			datasets[k.dataset] = di
			stats.Datasets = append(stats.Datasets, &adaptive_placementpb.DatasetStats{
				Tenant: uint32(ti),
				Name:   k.dataset,
			})
		}

		si, ok := shards[k.shard]
		if !ok {
			si = len(stats.Shards)
			shards[k.shard] = si
			stats.Shards = append(stats.Shards, &adaptive_placementpb.ShardStats{
				Id:    k.shard.id,
				Owner: k.shard.owner,
			})
		}

		ds := stats.Datasets[di]
		ds.Shards = append(ds.Shards, uint32(si))
		ds.Usage = append(ds.Usage, uint64(math.Round(c.ValueAt(now))))
	}

	for _, dataset := range stats.Datasets {
		c := d.counter(counterKey{
			tenant:  stats.Tenants[dataset.Tenant].TenantId,
			dataset: dataset.Name,
		})
		// Unlike the shard counters, we update the dataset-wide
		// counters at the build time.
		c.UpdateAt(float64(stdDev(dataset.Usage)), now)
		dataset.StdDev = uint64(math.Round(c.ValueAt(now)))
	}

	return stats
}
