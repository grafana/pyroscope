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

var DefaultPlacement = defaultPlacement{}

type defaultPlacement struct{}

func (defaultPlacement) NumTenantShards(Key, int) int { return 0 }

func (defaultPlacement) NumDatasetShards(Key, int) int { return 2 }

func (defaultPlacement) PickShard(k Key, n int) int { return int(k.Fingerprint % uint64(n)) }

func (defaultPlacement) Place(k Key) *Placement { return nil }

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
// Note that the instances are not guaranteed to be unique.
// It's also not guaranteed that the instances are available.
type Placement struct {
	Instances iter.Iterator[ring.InstanceDesc]
	Shard     uint32
}
