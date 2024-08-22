package placement

import (
	"github.com/grafana/dskit/ring"
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
	// NumTenantShards returns the number of shards
	// for a tenant from n total.
	NumTenantShards(k Key, n uint32) (size uint32)
	// NumDatasetShards returns the number of shards
	// for a dataset from n total.
	NumDatasetShards(k Key, n uint32) (size uint32)
	// PickShard returns the shard index
	// for a given key from n total.
	PickShard(k Key, n uint32) (shard uint32)
}

var DefaultPlacement = defaultPlacement{}

type defaultPlacement struct{}

func (defaultPlacement) NumTenantShards(Key, uint32) uint32 { return 0 }

func (defaultPlacement) NumDatasetShards(Key, uint32) uint32 { return 2 }

func (defaultPlacement) PickShard(k Key, n uint32) uint32 { return uint32(k.Fingerprint % uint64(n)) }

// Placement represents the placement for the given distribution key.
type Placement struct {
	// Note that the instances reference shared objects, and must not be modified.
	Instances []*ring.InstanceDesc
	Shard     uint32
}

// Next returns the next available location address.
func (p *Placement) Next() (instance *ring.InstanceDesc, ok bool) {
	for len(p.Instances) > 0 {
		instance, p.Instances = p.Instances[0], p.Instances[1:]
		if instance.State == ring.ACTIVE {
			return instance, true
		}
	}
	return nil, false
}
