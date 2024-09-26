package placement

import (
	"github.com/grafana/dskit/ring"

	"github.com/grafana/pyroscope/pkg/iter"
)

// Key represents the distribution key.
type Key struct {
	TenantID    string
	DatasetName string

	Tenant      uint64
	Dataset     uint64
	Fingerprint uint64
}

type Strategy interface {
	// Place returns the placement for the given key.
	// The method returns nil, if the placement is not
	// determined, and should be calculated by the caller.
	Place(k Key) *Placement
	// NumTenantShards returns the number of shards
	// for a tenant from n total.
	NumTenantShards(k Key, n int) (size int)
	// NumDatasetShards returns the number of shards
	// for a dataset from n total.
	NumDatasetShards(k Key, n int) (size int)
	// PickShard returns the shard index
	// for a given key from n total.
	PickShard(k Key, n int) (shard int)
}

// Placement represents the placement for a given key.
//
// Each key is mapped to one of the shards, based on the placement
// strategy. In turn, each shard is associated with an instance.
//
// Placement provides a number of instances that can host the key.
// It is assumed, that the caller will use the first one by default,
// and will try the rest in case of failure. This is done to avoid
// excessive data distribution in case of temporary unavailability
// of the instances: first, we try the instance that the key is
// mapped to, then we try the instances that host the dataset, then
// instances that host the tenant. Finally, we try any instances.
//
// Note that the instances are not guaranteed to be unique.
// It's also not guaranteed that the instances are available.
// Use ActiveInstances wrapper if you need to filter out inactive
// instances and duplicates.
type Placement struct {
	Instances iter.Iterator[ring.InstanceDesc]
	Shard     uint32
}

// ActiveInstances returns an iterator that filters out inactive
// instances and duplicates. Note that it does not mean that the
// instance is healthy.
func ActiveInstances(i iter.Iterator[ring.InstanceDesc]) iter.Iterator[ring.InstanceDesc] {
	seen := make(map[string]struct{})
	return FilterInstances(i, func(x *ring.InstanceDesc) bool {
		if _, ok := seen[x.Addr]; ok {
			return true
		}
		seen[x.Addr] = struct{}{}
		return x.State != ring.ACTIVE
	})
}

// FilterInstances returns an iterator that filters out instances
// on which the filter function returns true.
func FilterInstances(
	i iter.Iterator[ring.InstanceDesc],
	filter func(x *ring.InstanceDesc) bool,
) iter.Iterator[ring.InstanceDesc] {
	return &instances{
		Iterator: i,
		filter:   filter,
	}
}

type instances struct {
	iter.Iterator[ring.InstanceDesc]
	filter func(*ring.InstanceDesc) bool
	cur    ring.InstanceDesc
}

func (i *instances) At() ring.InstanceDesc { return i.cur }

func (i *instances) Next() bool {
	for i.Iterator.Next() {
		x := i.Iterator.At()
		if i.filter(&x) {
			continue
		}
		i.cur = x
		return true
	}
	return false
}
