package distributor

type PlacementStrategy interface {
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

func (defaultPlacement) NumTenantShards(string, uint32) uint64 { return 0 }

func (defaultPlacement) NumDatasetShards(Key, uint32) uint64 { return 2 }

func (defaultPlacement) PickShard(k Key, n uint32) uint32 { return uint32(k.Distribution % uint64(n)) }
