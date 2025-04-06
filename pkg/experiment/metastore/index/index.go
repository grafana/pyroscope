package index

import (
	"flag"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"go.etcd.io/bbolt"
	"golang.org/x/exp/maps"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
)

var ErrBlockExists = fmt.Errorf("block already exists")

type Config struct {
	ShardCacheSize      int `yaml:"shard_cache_size"`
	BlockWriteCacheSize int `yaml:"block_write_cache_size"`
	BlockReadCacheSize  int `yaml:"block_read_cache_size"`

	partitionDuration     time.Duration
	queryLookaroundPeriod time.Duration
}

var DefaultConfig = Config{
	ShardCacheSize:      2000,   // 128KB * 2000 = 256MB
	BlockReadCacheSize:  100000, // 8KB blocks = 800MB
	BlockWriteCacheSize: 10000,

	// FIXME(kolesnikovae): Do not modify, it will break the index.
	//
	// This parameter is not supported; used only for testing.
	// Partition key MUST be an input parameter.
	partitionDuration: 6 * time.Hour,

	// FIXME(kolesnikovae): Remove: build an interval tree.
	//
	// Currently, we do not use information about the time range of data each
	// partition refers to. For example, it's possible – though very unlikely
	// – for data from the past hour to be stored in a partition created a day
	// ago. We need to be cautious: when querying, we must identify all
	// partitions that may include the query time range. To ensure we catch
	// such "misplaced" data, we extend the query time range using this period.
	queryLookaroundPeriod: 24 * time.Hour,
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.IntVar(&cfg.ShardCacheSize, prefix+"shard-cache-size", DefaultConfig.ShardCacheSize, "Maximum number of shards to keep in memory")
	f.IntVar(&cfg.BlockWriteCacheSize, prefix+"block-write-cache-size", DefaultConfig.BlockWriteCacheSize, "Maximum number of written blocks to keep in memory")
	f.IntVar(&cfg.BlockReadCacheSize, prefix+"block-read-cache-size", DefaultConfig.BlockReadCacheSize, "Maximum number of read blocks to keep in memory")
	cfg.partitionDuration = DefaultConfig.partitionDuration
	cfg.queryLookaroundPeriod = DefaultConfig.queryLookaroundPeriod
}

type Store interface {
	CreateBuckets(*bbolt.Tx) error
	ListPartitions(*bbolt.Tx) ([]*store.Partition, error)
	LoadShard(*bbolt.Tx, store.PartitionKey, string, uint32) (*store.Shard, error)
}

type Index struct {
	logger     log.Logger
	config     Config
	store      Store
	partitions []*store.Partition
	shards     *shardCache
	blocks     *blockCache
	mu         sync.RWMutex
}

func NewIndex(logger log.Logger, s Store, cfg Config) *Index {
	return &Index{
		logger:     logger,
		config:     cfg,
		store:      s,
		partitions: make([]*store.Partition, 0),
		shards:     newShardCache(cfg.ShardCacheSize),
		blocks:     newBlockCache(cfg.BlockReadCacheSize, cfg.BlockWriteCacheSize),
	}
}

func NewStore() *store.IndexStore { return store.NewIndexStore() }

func (i *Index) Init(tx *bbolt.Tx) error { return i.store.CreateBuckets(tx) }

func (i *Index) Restore(tx *bbolt.Tx) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	var err error
	if i.partitions, err = i.store.ListPartitions(tx); err != nil {
		level.Error(i.logger).Log("msg", "failed to list partitions", "err", err)
		return err
	}

	// See comment in DefaultConfig.queryLookaroundPeriod.
	now := time.Now()
	high := now.Add(i.config.queryLookaroundPeriod)
	low := now.Add(-i.config.queryLookaroundPeriod)

	for _, p := range i.partitions {
		level.Info(i.logger).Log(
			"msg", "found metastore index partition",
			"timestamp", p.Key.Timestamp.Format(time.RFC3339),
			"duration", p.Key.Duration,
			"tenants", len(p.TenantShards),
		)
		if p.Key.Overlaps(low, high) {
			level.Info(i.logger).Log("msg", "loading partition in memory")
			var s *store.Shard
			for tenant, shards := range p.TenantShards {
				for shard := range shards {
					if s, err = i.store.LoadShard(tx, p.Key, tenant, shard); err != nil {
						level.Error(i.logger).Log(
							"msg", "failed to load tenant partition shard",
							"partition", p.Key,
							"tenant", tenant,
							"shard", shard,
							"err", err,
						)
						return err
					}
					if s != nil {
						i.shards.put(&indexShard{Shard: s})
					}
				}
			}
		}
	}

	level.Info(i.logger).Log("msg", "loaded metastore index partitions", "count", len(i.partitions))
	i.sortPartitions()
	return nil
}

func (i *Index) sortPartitions() {
	slices.SortFunc(i.partitions, func(a, b *store.Partition) int {
		return a.Compare(b)
	})
}

func (i *Index) InsertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	p := i.getOrCreatePartitionForBlock(b)
	s, err := i.getOrCreateShard(tx, p, metadata.Tenant(b), b.Shard)
	if err != nil {
		return err
	}
	i.blocks.put(s, b)
	return s.Store(tx, b)
}

func (i *Index) ReplaceBlocks(tx *bbolt.Tx, compacted *metastorev1.CompactedBlocks) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, b := range compacted.NewBlocks {
		s, err := i.getOrCreateShard(tx, i.getOrCreatePartitionForBlock(b), metadata.Tenant(b), b.Shard)
		if err != nil {
			return err
		}
		i.blocks.put(s, b)
		if err = s.Store(tx, b); err != nil {
			return err
		}
	}
	for k, list := range i.partitionedList(compacted.SourceBlocks) {
		s, err := i.getOrCreateShard(tx, i.getPartition(k), list.Tenant, list.Shard)
		if err != nil {
			return err
		}
		if s != nil {
			for _, b := range list.Blocks {
				i.blocks.delete(s, b)
			}
			if err = s.Delete(tx, list.Blocks...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (i *Index) GetBlocks(tx *bbolt.Tx, list *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error) {
	metas := make([]*metastorev1.BlockMeta, 0, len(list.Blocks))
	for k, partitioned := range i.partitionedList(list) {
		i.mu.RLock()
		s, err := i.getShard(tx, k, partitioned.Tenant, partitioned.Shard)
		i.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		if s != nil {
			for _, kv := range s.Find(tx, partitioned.Blocks...) {
				b := i.blocks.getOrCreate(s, kv).CloneVT()
				s.StringTable.Export(b)
				metas = append(metas, b)
			}
		}
	}
	return metas, nil
}

func (i *Index) GetTenantStats(tenant string) *metastorev1.TenantStats {
	stats := &metastorev1.TenantStats{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, p := range i.partitions {
		if !p.HasTenant(tenant) {
			continue
		}
		oldest := p.StartTime().UnixMilli()
		newest := p.EndTime().UnixMilli()
		stats.DataIngested = true
		if oldest < stats.OldestProfileTime {
			stats.OldestProfileTime = oldest
		}
		if newest > stats.NewestProfileTime {
			stats.NewestProfileTime = newest
		}
	}
	if !stats.DataIngested {
		return new(metastorev1.TenantStats)
	}

	return stats
}

func (i *Index) QueryMetadata(tx *bbolt.Tx, query MetadataQuery) ([]*metastorev1.BlockMeta, error) {
	q, err := newMetadataQuery(i, query)
	if err != nil {
		return nil, err
	}
	r, err := newBlockMetadataQuerier(tx, q).queryBlocks()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (i *Index) QueryMetadataLabels(tx *bbolt.Tx, query MetadataQuery) ([]*typesv1.Labels, error) {
	q, err := newMetadataQuery(i, query)
	if err != nil {
		return nil, err
	}
	r, err := newMetadataLabelQuerier(tx, q).queryLabels()
	if err != nil {
		return nil, err
	}
	return r.Labels(), nil
}

func (i *Index) getOrCreatePartitionForBlock(b *metastorev1.BlockMeta) *store.Partition {
	t := ulid.Time(ulid.MustParse(b.Id).Time())
	k := store.NewPartitionKey(t, i.config.partitionDuration)
	return i.getOrCreatePartition(k)
}

func (i *Index) getOrCreatePartition(k store.PartitionKey) *store.Partition {
	if p := i.getPartition(k); p != nil {
		return p
	}
	level.Debug(i.logger).Log("msg", "creating new metastore index partition", "key", k)
	p := store.NewPartition(k)
	i.partitions = append(i.partitions, p)
	i.sortPartitions()
	return p
}

func (i *Index) getPartition(key store.PartitionKey) *store.Partition {
	for _, p := range i.partitions {
		if p.Key.Equal(key) {
			return p
		}
	}
	return nil
}

func (i *Index) partitionedList(list *metastorev1.BlockList) map[store.PartitionKey]*metastorev1.BlockList {
	partitions := make(map[store.PartitionKey]*metastorev1.BlockList)
	for _, b := range list.Blocks {
		k := store.NewPartitionKey(ulid.Time(ulid.MustParse(b).Time()), i.config.partitionDuration)
		v := partitions[k]
		if v == nil {
			v = &metastorev1.BlockList{
				Shard:  list.Shard,
				Tenant: list.Tenant,
				Blocks: make([]string, 0, len(list.Blocks)),
			}
			partitions[k] = v
		}
		v.Blocks = append(v.Blocks, b)
	}
	return partitions
}

func (i *Index) getOrCreateShard(tx *bbolt.Tx, p *store.Partition, tenant string, shard uint32) (*store.Shard, error) {
	p.AddTenantShard(tenant, shard)
	x := i.shards.get(p.Key, tenant, shard)
	if x != nil && !x.readOnly {
		return x.Shard, nil
	}
	// If the shard is not found, or it is loaded for reads,
	// reload it and invalidate the cached version.
	s, err := i.store.LoadShard(tx, p.Key, tenant, shard)
	if err != nil {
		return nil, err
	}
	if s == nil {
		s = &store.Shard{
			Partition:   p.Key,
			Tenant:      tenant,
			Shard:       shard,
			StringTable: metadata.NewStringTable(),
		}
	}
	i.shards.put(&indexShard{
		Shard:    s,
		readOnly: false,
	})
	return s, nil
}

func (i *Index) getShard(tx *bbolt.Tx, p store.PartitionKey, tenant string, shard uint32) (*store.Shard, error) {
	x := i.shards.get(p, tenant, shard)
	if x != nil {
		return x.ShallowCopy(), nil
	}
	s, err := i.store.LoadShard(tx, p, tenant, shard)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}
	i.shards.put(&indexShard{
		Shard:    s,
		readOnly: true,
	})
	return s.ShallowCopy(), nil
}

type shardIterator struct {
	tx         *bbolt.Tx
	index      *Index
	tenants    []string
	partitions []*store.Partition
	shards     []*store.Shard
	cur        int
	err        error
}

func newShardIterator(tx *bbolt.Tx, index *Index, startTime, endTime time.Time, tenants ...string) *shardIterator {
	// See comment in DefaultConfig.queryLookaroundPeriod.
	startTime = startTime.Add(-index.config.queryLookaroundPeriod)
	endTime = endTime.Add(index.config.queryLookaroundPeriod)
	index.mu.RLock()
	defer index.mu.RUnlock()
	si := shardIterator{
		tx:         tx,
		partitions: make([]*store.Partition, 0, len(index.partitions)),
		tenants:    tenants,
		index:      index,
	}
	for _, p := range index.partitions {
		if !p.Overlaps(startTime, endTime) {
			continue
		}
		for _, t := range si.tenants {
			if p.HasTenant(t) {
				si.partitions = append(si.partitions, p)
				break
			}
		}
	}
	return &si
}

func (si *shardIterator) Err() error { return si.err }

func (si *shardIterator) At() *store.Shard { return si.shards[si.cur] }

func (si *shardIterator) Next() bool {
	if n := si.cur + 1; n < len(si.shards) {
		si.cur = n
		return true
	}
	si.cur = 0
	si.shards = si.shards[:0]
	for len(si.shards) == 0 && len(si.partitions) > 0 {
		si.loadPartition(si.partitions[0])
		si.partitions = si.partitions[1:]
	}
	return si.err == nil && si.cur < len(si.shards)
}

func (si *shardIterator) loadPartition(p *store.Partition) {
	for _, t := range si.tenants {
		si.loadShards(p, t)
	}
	slices.SortFunc(si.shards, compareShards)
	si.shards = slices.Compact(si.shards)
}

func (si *shardIterator) loadShards(p *store.Partition, tenant string) {
	si.index.mu.Lock()
	shards := maps.Keys(p.TenantShards[tenant])
	si.index.mu.Unlock()
	var err error
	var s *store.Shard
	for _, shard := range shards {
		si.index.mu.RLock()
		s, err = si.index.getShard(si.tx, p.Key, tenant, shard)
		si.index.mu.RUnlock()
		if err != nil {
			si.err = err
			return
		}
		if s != nil {
			si.shards = append(si.shards, s)
		}
	}
}

func compareShards(a, b *store.Shard) int {
	cmp := strings.Compare(a.Tenant, b.Tenant)
	if cmp == 0 {
		return int(a.Shard) - int(b.Shard)
	}
	return cmp
}
