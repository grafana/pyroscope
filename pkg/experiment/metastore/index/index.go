package index

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

const (
	partitionDuration = 6 * time.Hour
	// Indicates that partitions within this window are "protected" from being unloaded.
	partitionProtectionWindow = 30 * time.Minute
	partitionTenantCacheSize  = 32
)

var ErrBlockExists = fmt.Errorf("block already exists")

var DefaultConfig = Config{
	PartitionDuration:     partitionDuration,
	QueryLookaroundPeriod: partitionDuration,
	PartitionCacheSize:    partitionTenantCacheSize,
}

type Config struct {
	PartitionCacheSize    int `yaml:"partition_cache_size"`
	PartitionDuration     time.Duration
	QueryLookaroundPeriod time.Duration `yaml:"query_lookaround_period"`
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	// FIXME(kolesnikovae): This parameter is not fully supported.
	//  Overlapping partitions are difficult to handle correctly;
	//  without an interval tree, it may also be inefficient.
	//  Instead, we should consider immutable partition ranges:
	//  once a partition is created, all the keys targeting the
	//  time range of the partition should be directed to it.
	cfg.PartitionDuration = DefaultConfig.PartitionDuration
	f.IntVar(&cfg.PartitionCacheSize, prefix+"partition-cache-size", DefaultConfig.PartitionCacheSize, "How many partitions to keep loaded in memory per tenant.")
	f.DurationVar(&cfg.QueryLookaroundPeriod, prefix+"query-lookaround-period", DefaultConfig.QueryLookaroundPeriod, "")
}

type Store interface {
	CreateBuckets(*bbolt.Tx) error

	StoreBlock(tx *bbolt.Tx, shard *store.TenantShard, md *metastorev1.BlockMeta) error
	DeleteBlockList(*bbolt.Tx, store.PartitionKey, *metastorev1.BlockList) error

	ListPartitions(*bbolt.Tx) ([]*store.Partition, error)
	LoadTenantShard(*bbolt.Tx, store.PartitionKey, string, uint32) (*store.TenantShard, error)
}

type Index struct {
	logger log.Logger
	config *Config
	store  Store

	mu               sync.Mutex
	loadedPartitions map[tenantPartitionKey]*indexPartition
	partitions       []*store.Partition
}

type tenantPartitionKey struct {
	partition store.PartitionKey
	tenant    string
}

type indexPartition struct {
	partition  *store.Partition
	tenant     string
	shards     map[uint32]*indexShard
	accessedAt time.Time
}

type indexShard struct {
	blocks map[string]*metastorev1.BlockMeta
	*store.TenantShard
}

// NewIndex initializes a new metastore index.
//
// The index provides access to block metadata. The data is partitioned by time, shard and tenant. Partition identifiers
// contain the time period referenced by partitions, e.g., "20240923T16.1h" refers to a partition for the 1-hour period
// between 2024-09-23T16:00:00.000Z and 2024-09-23T16:59:59.999Z.
//
// Partitions are mostly transparent for the end user, though PartitionMeta is at times used externally. Partition
// durations are configurable (at application level).
//
// The index requires a backing Store for loading data in memory. Data is loaded directly via LoadPartitions() or when
// looking up blocks with FindBlock() or FindBlocksInRange().
func NewIndex(logger log.Logger, s Store, cfg *Config) *Index {
	// A fixed cache size gives us bounded memory footprint, however changes to the partition duration could reduce
	// the cache effectiveness.
	// TODO (aleks-p):
	//  - resize the cache at runtime when the config changes
	//  - consider auto-calculating the cache size to ensure we hold data for e.g., the last 24 hours
	return &Index{
		loadedPartitions: make(map[tenantPartitionKey]*indexPartition, cfg.PartitionCacheSize),
		partitions:       make([]*store.Partition, 0),
		store:            s,
		logger:           logger,
		config:           cfg,
	}
}

func NewStore() *store.IndexStore {
	return store.NewIndexStore()
}

func (i *Index) Init(tx *bbolt.Tx) error {
	return i.store.CreateBuckets(tx)
}

func (i *Index) Restore(tx *bbolt.Tx) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.partitions = nil
	clear(i.loadedPartitions)

	var err error
	i.partitions, err = i.store.ListPartitions(tx)
	if err != nil {
		return err
	}

	for _, p := range i.partitions {
		level.Info(i.logger).Log(
			"msg", "found metastore index partition",
			"timestamp", p.Key.Timestamp.Format(time.RFC3339),
			"duration", p.Key.Duration,
			"tenants", len(p.TenantShards),
		)
		if i.shouldKeepPartition(p) {
			level.Info(i.logger).Log("msg", "loading partition in memory")
			if err = i.loadPartition(tx, p); err != nil {
				return err
			}
		}
	}

	level.Info(i.logger).Log("msg", "loaded metastore index partitions", "count", len(i.partitions))
	i.sortPartitions()
	return nil
}

func (i *Index) shouldKeepPartition(p *store.Partition) bool {
	now := time.Now()
	low := now.Add(-partitionProtectionWindow)
	high := now.Add(partitionProtectionWindow)
	return p.Overlaps(low, high)
}

func (i *Index) sortPartitions() {
	slices.SortFunc(i.partitions, func(a, b *store.Partition) int {
		return a.Compare(b)
	})
}

func (i *Index) loadPartition(tx *bbolt.Tx, p *store.Partition) error {
	for tenant, shards := range p.TenantShards {
		partition := newIndexPartition(p, tenant)
		k := tenantPartitionKey{partition: p.Key, tenant: tenant}
		i.loadedPartitions[k] = partition
		for shard := range shards {
			s, err := i.store.LoadTenantShard(tx, p.Key, tenant, shard)
			if err != nil {
				level.Error(i.logger).Log("msg", "failed to load shard", "partition", p.Key.Timestamp, "shard", shard, "tenant", tenant)
				return err
			}
			partition.shards[shard] = newIndexShard(s)
		}
	}
	return nil
}

func (i *Index) InsertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.insertBlock(tx, b)
}

// insertBlock is the underlying implementation for inserting blocks. It is the caller's responsibility to enforce safe
// concurrent access. The method will create a new partition if needed.
func (i *Index) insertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	p := i.getOrCreatePartition(b)
	shard, err := i.getOrCreateTenantShard(tx, p, metadata.Tenant(b), b.Shard)
	if err != nil {
		return err
	}
	return shard.insert(tx, i.store, b)
}

func (i *Index) getOrCreatePartition(b *metastorev1.BlockMeta) *store.Partition {
	t := ulid.Time(ulid.MustParse(b.Id).Time())
	k := store.NewPartitionKey(t, i.config.PartitionDuration)
	p := i.findPartition(k)
	if p == nil {
		level.Debug(i.logger).Log("msg", "creating new metastore index partition", "key", k)
		p = store.NewPartition(k)
		i.partitions = append(i.partitions, p)
		i.sortPartitions()
	}
	return p
}

func (i *Index) findPartition(key store.PartitionKey) *store.Partition {
	for _, p := range i.partitions {
		if p.Key.Equal(key) {
			return p
		}
	}
	return nil
}

func (i *Index) getOrCreateTenantShard(tx *bbolt.Tx, p *store.Partition, tenant string, shard uint32) (*indexShard, error) {
	k := tenantPartitionKey{partition: p.Key, tenant: tenant}
	partition, ok := i.loadedPartitions[k]
	if !ok {
		i.unloadPartitions()
		partition = newIndexPartition(p, tenant)
		if err := partition.loadTenant(tx, i.store, tenant); err != nil {
			return nil, err
		}
		i.loadedPartitions[k] = partition
	}
	s, ok := partition.shards[shard]
	if !ok {
		s = newIndexShard(&store.TenantShard{
			Partition:   p.Key,
			Tenant:      tenant,
			Shard:       shard,
			StringTable: metadata.NewStringTable(),
		})
		partition.shards[shard] = s
		// This is the only way we "remember" the tenant shard.
		p.AddTenantShard(tenant, shard)
	}
	partition.accessedAt = time.Now()
	return s, nil
}

func (i *Index) FindBlocks(tx *bbolt.Tx, list *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	metas := make([]*metastorev1.BlockMeta, 0, len(list.Blocks))
	for k, partitioned := range i.partitionedList(list) {
		p := i.findPartition(k)
		if p == nil {
			continue
		}
		s, err := i.getOrLoadTenantShard(tx, p, partitioned.Tenant, partitioned.Shard)
		if err != nil {
			return nil, err
		}
		if s == nil {
			continue
		}
		for _, b := range partitioned.Blocks {
			if md := s.getBlock(b); md != nil {
				metas = append(metas, md)
			}
		}
	}
	return metas, nil
}

func (i *Index) partitionedList(list *metastorev1.BlockList) map[store.PartitionKey]*metastorev1.BlockList {
	partitions := make(map[store.PartitionKey]*metastorev1.BlockList)
	for _, b := range list.Blocks {
		k := store.NewPartitionKey(ulid.Time(ulid.MustParse(b).Time()), i.config.PartitionDuration)
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

func (i *Index) getOrLoadTenantShard(tx *bbolt.Tx, p *store.Partition, tenant string, shard uint32) (*indexShard, error) {
	// Check if we've seen any data for the tenant shard at all.
	shards, ok := p.TenantShards[tenant]
	if !ok {
		return nil, nil
	}
	if _, ok = shards[shard]; !ok {
		return nil, nil
	}
	k := tenantPartitionKey{partition: p.Key, tenant: tenant}
	partition, ok := i.loadedPartitions[k]
	if !ok {
		// Read from store.
		partition = newIndexPartition(p, tenant)
		if err := partition.loadTenant(tx, i.store, tenant); err != nil {
			return nil, err
		}
		if len(partition.shards) == 0 {
			return nil, nil
		}
		i.unloadPartitions()
		i.loadedPartitions[k] = partition
	}
	partition.accessedAt = time.Now()
	return partition.shards[shard], nil
}

// ReplaceBlocks removes source blocks from the index and inserts replacement blocks into the index. The intended usage
// is for block compaction. The replacement blocks could be added to the same or a different partition.
func (i *Index) ReplaceBlocks(tx *bbolt.Tx, compacted *metastorev1.CompactedBlocks) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, b := range compacted.NewBlocks {
		if err := i.insertBlock(tx, b); err != nil {
			if errors.Is(err, ErrBlockExists) {
				continue
			}
			return err
		}
	}
	return i.deleteBlockList(tx, compacted.SourceBlocks)
}

func (i *Index) deleteBlockList(tx *bbolt.Tx, list *metastorev1.BlockList) error {
	for k, partitioned := range i.partitionedList(list) {
		if err := i.store.DeleteBlockList(tx, k, partitioned); err != nil {
			return err
		}
		p := i.findPartition(k)
		if p == nil {
			continue
		}
		s, err := i.getOrLoadTenantShard(tx, p, partitioned.Tenant, partitioned.Shard)
		if err != nil {
			return err
		}
		if s == nil {
			continue
		}
		for _, b := range partitioned.Blocks {
			delete(s.blocks, b)
		}
	}
	return nil
}

func (i *Index) unloadPartitions() {
	tenantPartitions := make(map[string][]*indexPartition)
	excessPerTenant := make(map[string]int)
	for k, p := range i.loadedPartitions {
		tenantPartitions[k.tenant] = append(tenantPartitions[k.tenant], p)
		if len(tenantPartitions[k.tenant]) > i.config.PartitionCacheSize {
			excessPerTenant[k.tenant]++
		}
	}

	for t, partitions := range tenantPartitions {
		toRemove, ok := excessPerTenant[t]
		if !ok {
			continue
		}
		slices.SortFunc(partitions, func(a, b *indexPartition) int {
			return a.accessedAt.Compare(b.accessedAt)
		})
		level.Debug(i.logger).Log("msg", "unloading metastore index partitions", "tenant", t, "to_remove", len(partitions))
		for _, p := range partitions {
			if i.shouldKeepPartition(p.partition) {
				continue
			}
			level.Debug(i.logger).Log("unloading metastore index partition", "key", p.partition.Key, "accessed_at", p.accessedAt.Format(time.RFC3339))
			k := tenantPartitionKey{
				partition: p.partition.Key,
				tenant:    t,
			}
			delete(i.loadedPartitions, k)
			toRemove--
			if toRemove == 0 {
				break
			}
		}
	}
}

func (i *Index) GetTenantStats(tenant string) *metastorev1.TenantStats {
	stats := &metastorev1.TenantStats{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}

	i.mu.Lock()
	defer i.mu.Unlock()

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

// TODO(kolesnikovae): We query meta with the mutex held, which
//  will cause contention and latency issues. Fix it once we make
//  locks more granular (partition-tenant-shard level).

func (i *Index) QueryMetadata(tx *bbolt.Tx, query MetadataQuery) iter.Iterator[*metastorev1.BlockMeta] {
	q, err := newMetadataQuery(i, query)
	if err != nil {
		return iter.NewErrIterator[*metastorev1.BlockMeta](err)
	}
	i.mu.Lock()
	metas, err := iter.Slice[*metastorev1.BlockMeta](newBlockMetadataIterator(tx, q))
	i.mu.Unlock()
	if err != nil {
		return iter.NewErrIterator[*metastorev1.BlockMeta](err)
	}
	return iter.NewSliceIterator(metas)
}

func (i *Index) QueryMetadataLabels(tx *bbolt.Tx, query MetadataQuery) ([]*typesv1.Labels, error) {
	q, err := newMetadataQuery(i, query)
	if err != nil {
		return nil, err
	}
	i.mu.Lock()
	r, err := newMetadataLabelQuerier(tx, q).queryLabels()
	i.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return r.Labels(), nil
}

func newIndexPartition(p *store.Partition, tenant string) *indexPartition {
	return &indexPartition{
		partition:  p,
		tenant:     tenant,
		shards:     make(map[uint32]*indexShard),
		accessedAt: time.Now(),
	}
}

func (p *indexPartition) loadTenant(tx *bbolt.Tx, store Store, tenant string) error {
	for shard := range p.partition.TenantShards[tenant] {
		s, err := store.LoadTenantShard(tx, p.partition.Key, tenant, shard)
		if err != nil {
			return err
		}
		if len(s.Blocks) > 0 {
			p.shards[shard] = newIndexShard(s)
		}
	}
	return nil
}

func newIndexShard(s *store.TenantShard) *indexShard {
	x := &indexShard{
		blocks:      make(map[string]*metastorev1.BlockMeta),
		TenantShard: s,
	}
	for _, md := range s.Blocks {
		x.blocks[md.Id] = md
	}
	return x
}

func (s *indexShard) insert(tx *bbolt.Tx, x Store, md *metastorev1.BlockMeta) error {
	if _, ok := s.blocks[md.Id]; ok {
		return ErrBlockExists
	}
	s.blocks[md.Id] = md
	return x.StoreBlock(tx, s.TenantShard, md)
}

func (s *indexShard) getBlock(blockID string) *metastorev1.BlockMeta {
	md, ok := s.blocks[blockID]
	if !ok {
		return nil
	}
	mdCopy := md.CloneVT()
	s.TenantShard.StringTable.Export(mdCopy)
	return mdCopy
}
