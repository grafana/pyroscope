package index

import (
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
	kvstore "github.com/grafana/pyroscope/pkg/experiment/metastore/store"
)

type indexShard struct {
	*store.Shard
	readOnly bool
}

type shardCache struct {
	mu     sync.RWMutex
	shards *lru.TwoQueueCache[shardCacheKey, *indexShard]
}

type shardCacheKey struct {
	partition store.PartitionKey
	tenant    string
	shard     uint32
}

type blockCache struct {
	mu    sync.RWMutex
	reads *lru.TwoQueueCache[blockCacheKey, *metastorev1.BlockMeta]
	write *lru.Cache[blockCacheKey, *metastorev1.BlockMeta]
}

type blockCacheKey struct {
	tenant string
	shard  uint32
	block  string
}

func newShardCache(size int) *shardCache {
	c, _ := lru.New2Q[shardCacheKey, *indexShard](size)
	return &shardCache{
		shards: c,
	}
}

func (c *shardCache) get(p store.PartitionKey, tenant string, shard uint32) *indexShard {
	k := shardCacheKey{
		partition: p,
		tenant:    tenant,
		shard:     shard,
	}
	v, _ := c.shards.Get(k)
	return v
}

func (c *shardCache) put(s *indexShard) {
	k := shardCacheKey{
		partition: s.Shard.Partition,
		tenant:    s.Shard.Tenant,
		shard:     s.Shard.Shard,
	}
	c.shards.Add(k, s)
}

func newBlockCache(rcs, wcs int) *blockCache {
	reads, _ := lru.New2Q[blockCacheKey, *metastorev1.BlockMeta](rcs)
	write, _ := lru.New[blockCacheKey, *metastorev1.BlockMeta](wcs)
	return &blockCache{
		reads: reads,
		write: write,
	}
}

func (c *blockCache) getOrCreate(shard *store.Shard, block kvstore.KV) *metastorev1.BlockMeta {
	k := blockCacheKey{
		tenant: shard.Tenant,
		shard:  shard.Shard,
		block:  string(block.Key),
	}
	c.mu.RLock()
	v, ok := c.reads.Get(k)
	if ok {
		c.mu.RUnlock()
		return v
	}
	v, ok = c.write.Get(k)
	if ok {
		c.mu.RUnlock()
		return v
	}
	c.mu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok = c.reads.Get(k)
	if ok {
		return v
	}
	v, ok = c.write.Get(k)
	if ok {
		return v
	}
	var md metastorev1.BlockMeta
	if err := md.UnmarshalVT(block.Value); err != nil {
		return &md
	}
	c.reads.Add(k, &md)
	return &md
}

func (c *blockCache) put(shard *store.Shard, md *metastorev1.BlockMeta) {
	k := blockCacheKey{
		tenant: shard.Tenant,
		shard:  shard.Shard,
		block:  md.Id,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if md.CompactionLevel >= 2 {
		c.reads.Add(k, md)
		return
	}
	c.write.Add(k, md)
}

func (c *blockCache) delete(shard *store.Shard, block string) {
	k := blockCacheKey{
		tenant: shard.Tenant,
		shard:  shard.Shard,
		block:  block,
	}
	c.write.Remove(k)
	c.reads.Remove(k)
}
