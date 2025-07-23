package index

import (
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/pkg/metastore/index/store"
	kvstore "github.com/grafana/pyroscope/pkg/metastore/store"
)

// Shard cache.
//
// The cache helps us to avoid repeatedly reading the string table from
// the persistent store. Cached shards have a flag that indicates whether
// the shard was loaded for reads. Any write operation should invalidate
// the cached entry and reload it as it may violate transaction isolation.
//
// Writes are always sequential and never concurrent. Therefore, it's
// guaranteed that every write operation observes the latest state of
// the shard on disk. The cache introduces a possibility to observe
// a stale state in the cache, because of the concurrent reads that
// share the same cache.
//
// Reads are concurrent and may run in transactions that began before
// the ongoing write transaction. If a read transaction reads the shard
// state from the disk, its state is obsolete from the writer perspective,
// since it corresponds to an older transaction; if such state is cached,
// all participants may observe it. Therefore, we mark such shards as
// read-only to let the writer know about it.
//
// Reads may observe a state modified "in the future", by a write
// transaction that has started after the read transaction. This is fine,
// as "stale reads" are resolved at the raft level. It is not fine,
// however, if the write transaction uses the cached shard state, loaded
// by read transaction.
type shardCache struct {
	mu    sync.RWMutex
	cache *lru.TwoQueueCache[shardCacheKey, *indexShardCached]
	store Store
}

type shardCacheKey struct {
	partition indexstore.Partition
	tenant    string
	shard     uint32
}

type indexShardCached struct {
	*indexstore.Shard
	readOnly bool
}

func newShardCache(size int, s Store) *shardCache {
	if size <= 0 {
		size = 1
	}
	c, _ := lru.New2Q[shardCacheKey, *indexShardCached](size)
	return &shardCache{cache: c, store: s}
}

func (c *shardCache) update(tx *bbolt.Tx, p indexstore.Partition, tenant string, shard uint32, fn func(*indexstore.Shard) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.getForWriteUnsafe(tx, p, tenant, shard)
	if err != nil {
		return err
	}
	return fn(s)
}

func (c *shardCache) getForWrite(tx *bbolt.Tx, p indexstore.Partition, tenant string, shard uint32) (*indexstore.Shard, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getForWriteUnsafe(tx, p, tenant, shard)
}

func (c *shardCache) getForWriteUnsafe(tx *bbolt.Tx, p indexstore.Partition, tenant string, shard uint32) (*indexstore.Shard, error) {
	k := shardCacheKey{
		partition: p,
		tenant:    tenant,
		shard:     shard,
	}
	x, found := c.cache.Get(k)
	if found && x != nil && !x.readOnly {
		return x.Shard, nil
	}
	// If the shard is not found, or it is loaded for reads,
	// reload it and invalidate the cached version.
	s, err := c.store.LoadShard(tx, p, tenant, shard)
	if err != nil {
		return nil, err
	}
	if s == nil {
		s = indexstore.NewShard(p, tenant, shard)
	}
	c.cache.Add(k, &indexShardCached{
		Shard:    s,
		readOnly: false,
	})
	return s, nil
}

func (c *shardCache) getForRead(tx *bbolt.Tx, p indexstore.Partition, tenant string, shard uint32) (*indexstore.Shard, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := shardCacheKey{
		partition: p,
		tenant:    tenant,
		shard:     shard,
	}
	x, found := c.cache.Get(k)
	if found && x != nil {
		return x.ShallowCopy(), nil
	}
	s, err := c.store.LoadShard(tx, p, tenant, shard)
	if err != nil {
		return nil, err
	}
	if s == nil {
		// Returning an empty shard is fine, as this
		// is a read operation.
		return indexstore.NewShard(p, tenant, shard), nil
	}
	c.cache.Add(k, &indexShardCached{
		Shard:    s,
		readOnly: true,
	})
	return s, nil
}

func (c *shardCache) delete(p indexstore.Partition, tenant string, shard uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := shardCacheKey{partition: p, tenant: tenant, shard: shard}
	c.cache.Remove(k)
}

// Block cache.
//
// Metadata entries might be large, tens of kilobytes, depending on the number
// of datasets, labels, and other metadata. Therefore, we use block cache
// to avoid repeatedly decoding the serialized raw bytes. The cache does not
// require any special coordination, as it is accessed by keys, which are
// loaded from the disk in the current transaction.
//
// The cache is split into two parts: read and write. This is done to prevent
// cache pollution in case of compaction delays.
//
// The read cache is populated with blocks that are fully compacted and with
// blocks queried by the user. We use 2Q cache replacement strategy to ensure
// that the most recently read blocks are kept in memory, while frequently
// accessed older blocks are not evicted prematurely.
//
// The write cache is used to store blocks that are being written to the index.
// It is important because it's guaranteed that the block will be read soon for
// compaction. The write cache is accessed for reads if the read cache does not
// contain the block queried.
type blockCache struct {
	mu    sync.RWMutex
	read  *lru.TwoQueueCache[blockCacheKey, *metastorev1.BlockMeta]
	write *lru.Cache[blockCacheKey, *metastorev1.BlockMeta]
}

type blockCacheKey struct {
	tenant string
	shard  uint32
	block  string
}

func newBlockCache(rcs, wcs int) *blockCache {
	var c blockCache
	if rcs <= 0 {
		rcs = 1
	}
	if wcs <= 0 {
		wcs = 1
	}
	c.read, _ = lru.New2Q[blockCacheKey, *metastorev1.BlockMeta](rcs)
	c.write, _ = lru.New[blockCacheKey, *metastorev1.BlockMeta](wcs)
	return &c
}

func (c *blockCache) getOrCreate(shard *indexstore.Shard, block kvstore.KV) *metastorev1.BlockMeta {
	k := blockCacheKey{
		tenant: shard.Tenant,
		shard:  shard.Shard,
		block:  string(block.Key),
	}
	c.mu.RLock()
	v, ok := c.read.Get(k)
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
	v, ok = c.read.Get(k)
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
	c.read.Add(k, &md)
	return &md
}

func (c *blockCache) put(shard *indexstore.Shard, md *metastorev1.BlockMeta) {
	k := blockCacheKey{
		tenant: shard.Tenant,
		shard:  shard.Shard,
		block:  md.Id,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if md.CompactionLevel >= 2 {
		c.read.Add(k, md)
		return
	}
	c.write.Add(k, md)
}

func (c *blockCache) delete(shard *indexstore.Shard, block string) {
	k := blockCacheKey{
		tenant: shard.Tenant,
		shard:  shard.Shard,
		block:  block,
	}
	c.write.Remove(k)
	c.read.Remove(k)
}
