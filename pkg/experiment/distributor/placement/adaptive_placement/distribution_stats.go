package adaptive_placement

import (
	"strings"
	"sync"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/ewma"
)

type DistributionStats struct {
	mu        sync.Mutex
	dict      *dict
	counters  map[key]*ewma.Rate
	window    time.Duration
	retention time.Duration
}

func (t *DistributionStats) RecordStats(md *metastorev1.BlockMeta) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sk := shard{
		owner: t.dict.get(""), // TODO: md.CreatedBy
		id:    md.Shard,
	}
	now := time.Now().UnixNano()
	for _, d := range md.Datasets {
		c := t.counter(key{
			tenant:  t.dict.get(d.TenantId),
			dataset: t.dict.get(d.Name),
			shard:   sk,
		})
		c.UpdateAt(float64(d.Size), now)
	}
}

func (t *DistributionStats) counter(k key) *ewma.Rate {
	c, ok := t.counters[k]
	if !ok {
		c = ewma.NewHalfLife(t.window)
		t.counters[k] = c
	}
	return c
}

type key struct {
	tenant  *dictValue
	dataset *dictValue
	shard   shard
}

type shard struct {
	owner *dictValue
	id    uint32
}

type dict struct{ m map[string]*dictValue }

type dictValue struct {
	val  string
	refs uint32
}

func (d *dict) get(x string) *dictValue {
	v, ok := d.m[x]
	if !ok {
		s := strings.Clone(x)
		v = &dictValue{val: s}
		d.m[s] = v
	}
	v.refs++
	return v
}

func (d *dict) rem(v *dictValue) {
	v.refs--
	if v.refs <= 0 {
		delete(d.m, v.val)
	}
}

func (t *DistributionStats) UpdateStats() *adaptive_placementpb.DistributionStats {
	var s adaptive_placementpb.DistributionStats
	tenants := make(map[string]int)
	datasets := make(map[string]int)
	shards := make(map[shard]int)

	for k, c := range t.counters {
		if time.Since(c.LastUpdate()) > t.retention {
			t.dict.rem(k.tenant)
			t.dict.rem(k.dataset)
			t.dict.rem(k.shard.owner)
			continue
		}

		ti, ok := tenants[k.tenant.val]
		if !ok {
			ti = len(s.Tenants)
			tenants[k.tenant.val] = ti
			s.Tenants = append(s.Tenants, &adaptive_placementpb.TenantStats{
				TenantId: k.tenant.val,
			})
		}

		di, ok := datasets[k.dataset.val]
		if !ok {
			di = len(s.Datasets)
			datasets[k.dataset.val] = di
			s.Datasets = append(s.Datasets, &adaptive_placementpb.DatasetStats{
				Tenant: uint32(ti),
				Name:   k.dataset.val,
			})
		}

		si, ok := shards[k.shard]
		if !ok {
			si = len(s.Shards)
			shards[k.shard] = si
			s.Shards = append(s.Shards, &adaptive_placementpb.ShardStats{
				Id:    k.shard.id,
				Owner: k.shard.owner.val,
			})
		}

		ds := s.Datasets[di]
		ds.Shards = append(ds.Shards, uint32(si))
		ds.Usage = append(ds.Usage, uint32(c.Value()))

		t.dict.rem(k.tenant)
		t.dict.rem(k.dataset)
		t.dict.rem(k.shard.owner)
	}

	return &s
}
