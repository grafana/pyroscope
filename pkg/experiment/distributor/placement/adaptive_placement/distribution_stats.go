package adaptive_placement

import (
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/ewma"
)

// DistributionStats is a helper struct that tracks the
// data rate of each dataset based on the metadata
// records.
type DistributionStats struct {
	mu        sync.Mutex
	counters  map[counterKey]*ewma.Rate
	window    time.Duration
	retention time.Duration
}

func NewDistributionStats(window, retention time.Duration) *DistributionStats {
	return &DistributionStats{
		counters:  make(map[counterKey]*ewma.Rate),
		window:    window,
		retention: retention,
	}
}

func (d *DistributionStats) RecordStats(md *metastorev1.BlockMeta, now int64) {
	if isStale(md.Id, now, d.retention.Nanoseconds()) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	// TODO(kolesnikovae): intern strings with unique (go 1.23)
	sk := shard{
		id:    md.Shard,
		owner: "", // TODO: md.CreatedBy
	}
	for _, ds := range md.Datasets {
		c := d.counter(counterKey{
			tenant:  ds.TenantId,
			dataset: ds.Name,
			shard:   sk,
		})
		c.UpdateAt(float64(ds.Size), now)
	}
}

func isStale(s string, now, retention int64) bool {
	id, err := ulid.Parse(s)
	if err != nil {
		return true
	}
	t := int64(id.Time() * 1e6) // ms -> ns
	d := now - t
	// It's possible that the block was created
	// "in the future" because of the time skew.
	if d < 0 {
		return false
	}
	// Otherwise, filter out blocks that are too old.
	return d > retention
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

func compareKeys(a, b counterKey) int { return a.compare(b) }

type shard struct {
	owner string
	id    uint32
}

func (d *DistributionStats) Build(now int64) *adaptive_placementpb.DistributionStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	var s adaptive_placementpb.DistributionStats
	tenants := make(map[string]int)
	datasets := make(map[string]int)
	shards := make(map[shard]int)

	// Although, not strictly required, we iterate over the keys
	// in a deterministic order to make the output deterministic.
	keys := make([]counterKey, 0, len(d.counters))
	for k := range d.counters {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareKeys)

	for _, k := range keys {
		c := d.counters[k]
		age := now - c.LastUpdate().UnixNano()
		if age >= d.retention.Nanoseconds() {
			delete(d.counters, k)
			continue
		}
		// Skip dataset-wide counters.
		if k.shard.id == 0 {
			continue
		}

		ti, ok := tenants[k.tenant]
		if !ok {
			ti = len(s.Tenants)
			tenants[k.tenant] = ti
			s.Tenants = append(s.Tenants, &adaptive_placementpb.TenantStats{
				TenantId: k.tenant,
			})
		}

		di, ok := datasets[k.dataset]
		if !ok {
			di = len(s.Datasets)
			datasets[k.dataset] = di
			s.Datasets = append(s.Datasets, &adaptive_placementpb.DatasetStats{
				Tenant: uint32(ti),
				Name:   k.dataset,
			})
		}

		si, ok := shards[k.shard]
		if !ok {
			si = len(s.Shards)
			shards[k.shard] = si
			s.Shards = append(s.Shards, &adaptive_placementpb.ShardStats{
				Id:    k.shard.id,
				Owner: k.shard.owner,
			})
		}

		ds := s.Datasets[di]
		ds.Shards = append(ds.Shards, uint32(si))
		ds.Usage = append(ds.Usage, uint64(math.Round(c.ValueAt(now))))
	}

	for _, dataset := range s.Datasets {
		c := d.counter(counterKey{
			tenant:  s.Tenants[dataset.Tenant].TenantId,
			dataset: dataset.Name,
		})
		c.UpdateAt(float64(stdDev(dataset.Usage)), now)
		dataset.StdDev = uint64(math.Round(c.ValueAt(now)))
	}

	return &s
}
