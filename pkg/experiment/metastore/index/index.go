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
)

const (
	partitionDuration = 6 * time.Hour
	// Indicates that partitions within this window are "protected" from being unloaded.
	partitionProtectionWindow = 24 * time.Hour
	partitionTenantCacheSize  = 32
)

var (
	ErrBlockExists = fmt.Errorf("block already exists")
	ErrReadAborted = fmt.Errorf("read aborted")
)

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
	config Config
	store  Store

	// The global lock protects index partitions.
	// Index partition shards have their own locks.
	global sync.Mutex
	// TODO: cache at the shard level.
	tenantPartitions map[tenantPartitionKey]*indexPartition
	partitions       []*store.Partition

	// We need to ensure that the replacement is atomic, but it may span
	// multiple partitions and tenants (and, theoretically, shards).
	// The mutex synchronizes queries with replacements; insertions are not
	// affected but require synchronization via the global and shard locks.
	// No queries are allowed during the replacement and vice versa.
	//
	// The lock should be taken before the global lock.
	replace sync.RWMutex

	// The function reports true if an existent partition should
	// be evicted when a new one is added. It's used in tests exclusively.
	keep func(*store.Partition) bool
}

type tenantPartitionKey struct {
	partition store.PartitionKey
	tenant    string
}

type indexPartition struct {
	partition  *store.Partition
	tenant     string
	accessedAt time.Time
	modifyTxn  int
	loaded     bool
	shards     map[uint32]*indexShard
}

type indexShard struct {
	mu     sync.RWMutex
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
func NewIndex(logger log.Logger, s Store, cfg Config) *Index {
	// A fixed cache size gives us bounded memory footprint, however changes to the partition duration could reduce
	// the cache effectiveness.
	// TODO (aleks-p):
	//  - resize the cache at runtime when the config changes
	//  - consider auto-calculating the cache size to ensure we hold data for e.g., the last 24 hours
	idx := Index{
		tenantPartitions: make(map[tenantPartitionKey]*indexPartition, cfg.PartitionCacheSize),
		partitions:       make([]*store.Partition, 0),
		store:            s,
		logger:           logger,
		config:           cfg,
	}
	idx.keep = idx.shouldKeepPartition
	return &idx
}

func NewStore() *store.IndexStore {
	return store.NewIndexStore()
}

func (i *Index) Init(tx *bbolt.Tx) error {
	return i.store.CreateBuckets(tx)
}

func (i *Index) Restore(tx *bbolt.Tx) error {
	i.global.Lock()
	defer i.global.Unlock()

	i.partitions = nil
	clear(i.tenantPartitions)

	var err error
	i.partitions, err = i.store.ListPartitions(tx)
	if err != nil {
		level.Error(i.logger).Log("msg", "failed to list partitions", "err", err)
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
			for tenant := range p.TenantShards {
				k := tenantPartitionKey{partition: p.Key, tenant: tenant}
				partition := newIndexPartition(p, tenant)
				partition.accessedAt = time.Now()
				partition.modifyTxn = tx.ID()
				i.tenantPartitions[k] = partition
				if err = partition.load(tx, i.store, tenant); err != nil {
					level.Error(i.logger).Log(
						"msg", "failed to load tenant partition",
						"partition", p.Key,
						"tenant", tenant,
						"err", err,
					)
					return err
				}
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

func (i *Index) InsertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	i.global.Lock()
	defer i.global.Unlock()
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
	p := i.getPartition(k)
	if p == nil {
		level.Debug(i.logger).Log("msg", "creating new metastore index partition", "key", k)
		p = store.NewPartition(k)
		i.partitions = append(i.partitions, p)
		i.sortPartitions()
	}
	return p
}

func (i *Index) getPartition(key store.PartitionKey) *store.Partition {
	for _, p := range i.partitions {
		// TODO: Binary search.
		if p.Key.Equal(key) {
			return p
		}
	}
	return nil
}

func (i *Index) getOrCreateTenantShard(tx *bbolt.Tx, p *store.Partition, tenant string, shard uint32) (*indexShard, error) {
	k := tenantPartitionKey{partition: p.Key, tenant: tenant}
	partition, ok := i.tenantPartitions[k]
	if !ok {
		partition = newIndexPartition(p, tenant)
		i.tenantPartitions[k] = partition
	}
	if err := partition.load(tx, i.store, tenant); err != nil {
		return nil, err
	}
	s, ok := partition.shards[shard]
	if !ok {
		s = newIndexShard(&store.TenantShard{
			Partition:   p.Key,
			Tenant:      tenant,
			Shard:       shard,
			StringTable: metadata.NewStringTable(),
		})
		// This is the only way we "remember" the tenant shard.
		partition.shards[shard] = s
		p.AddTenantShard(tenant, shard)
	}
	partition.accessedAt = time.Now()
	// This is a writable transaction (getOrCreate),
	// therefore we assume the partition is modified.
	partition.modifyTxn = tx.ID()
	i.unloadPartitions(tx)
	return s, nil
}

func (i *Index) getOrLoadTenantShard(tx *bbolt.Tx, p *store.Partition, tenant string, shard uint32) (*indexShard, error) {
	shards, ok := p.TenantShards[tenant]
	if !ok {
		return nil, nil
	}
	if _, ok = shards[shard]; !ok {
		return nil, nil
	}
	k := tenantPartitionKey{partition: p.Key, tenant: tenant}
	partition, ok := i.tenantPartitions[k]
	if !ok {
		partition = newIndexPartition(p, tenant)
		i.tenantPartitions[k] = partition
	}
	if err := partition.load(tx, i.store, tenant); err != nil {
		return nil, err
	}
	s := partition.shards[shard]
	partition.accessedAt = time.Now()
	partition.modifyTxn = tx.ID()
	i.unloadPartitions(tx)
	return s, nil
}

func (i *Index) GetBlocks(tx *bbolt.Tx, list *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error) {
	partitionedLists := i.partitionedList(list)
	metas := make([]*metastorev1.BlockMeta, 0, len(list.Blocks))
	// Since both GetBlocks and Replace may cover multiple partitions,
	// we need to synchronize access; otherwise, it is theoretically
	// possible that the function will see partial results of the
	// replacement (in practice, this should not be the case because
	// the functions operate on not-overlapping sets).
	i.replace.RLock()
	defer i.replace.RUnlock()
	for k, partitioned := range partitionedLists {
		s, err := i.getIndexShard(tx, k, partitioned)
		if err != nil {
			return nil, err
		}
		if s != nil {
			metas = s.getBlocks(metas, partitioned.Blocks...)
		}
	}
	return metas, nil
}

func (i *Index) getIndexShard(
	tx *bbolt.Tx,
	key store.PartitionKey,
	list *metastorev1.BlockList,
) (*indexShard, error) {
	i.global.Lock()
	defer i.global.Unlock()
	if p := i.getPartition(key); p != nil {
		return i.getOrLoadTenantShard(tx, p, list.Tenant, list.Shard)
	}
	return nil, nil
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

// ReplaceBlocks removes source blocks from the index and inserts replacement blocks into the index. The intended usage
// is for block compaction. The replacement blocks could be added to the same or a different partition.
func (i *Index) ReplaceBlocks(tx *bbolt.Tx, compacted *metastorev1.CompactedBlocks) error {
	// This is meant to be a relatively rare (tens per second) and not very slow
	// operation, therefore taking a lock here should not affect insertion.
	i.replace.Lock()
	defer i.replace.Unlock()

	i.global.Lock()
	defer i.global.Unlock()

	for _, b := range compacted.NewBlocks {
		p := i.getOrCreatePartition(b)
		shard, err := i.getOrCreateTenantShard(tx, p, metadata.Tenant(b), b.Shard)
		if err != nil {
			return err
		}
		switch err = shard.insert(tx, i.store, b); {
		case err == nil:
		case errors.Is(err, ErrBlockExists):
		default:
			return err
		}
	}

	for k, partitioned := range i.partitionedList(compacted.SourceBlocks) {
		if err := i.store.DeleteBlockList(tx, k, partitioned); err != nil {
			return err
		}
		p := i.getPartition(k)
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
		s.delete(partitioned.Blocks...)
	}

	return nil
}

func (i *Index) unloadPartitions(tx *bbolt.Tx) {
	if tx.DB().Stats().OpenTxN > 0 {
		// A transaction may be opened right after the check.
		// The reader must ensure that the partition to load has
		// not been modified after the transaction has begun.
		return
	}

	tenants := make(map[string][]*indexPartition)
	excessPerTenant := make(map[string]int)
	for k, p := range i.tenantPartitions {
		tenants[k.tenant] = append(tenants[k.tenant], p)
		if len(tenants[k.tenant]) > i.config.PartitionCacheSize {
			excessPerTenant[k.tenant]++
		}
	}

	for tenant, partitions := range tenants {
		rem, ok := excessPerTenant[tenant]
		if !ok {
			continue
		}
		slices.SortFunc(partitions, func(a, b *indexPartition) int {
			return a.accessedAt.Compare(b.accessedAt)
		})
		level.Debug(i.logger).Log("msg", "unloading metastore index partitions", "tenant", tenant, "to_remove", len(partitions))
		for _, p := range partitions {
			if p.modifyTxn >= tx.ID() {
				// We must never unload a partition that was modified
				// in the current transaction, or in a transaction
				// that has begun after the current transaction.
				continue
			}
			if i.keep(p.partition) {
				// The default keep check reports false if the partition
				// timestamp is far from the current time and the number
				// of tenant partitions loaded into memory exceeds the limit.
				continue
			}
			level.Debug(i.logger).Log("unloading metastore index partition", "key", p.partition.Key, "accessed_at", p.accessedAt.Format(time.RFC3339))
			partition, ok := i.tenantPartitions[tenantPartitionKey{partition: p.partition.Key, tenant: tenant}]
			if !ok {
				// It's expected that some partitions have not been loaded.
				continue
			}
			if !partition.loaded {
				continue
			}
			partition.unload()
			rem--
			if rem == 0 {
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

	i.global.Lock()
	defer i.global.Unlock()

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

	i.replace.RLock()
	defer i.replace.RUnlock()
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
	i.replace.RLock()
	defer i.replace.RUnlock()
	r, err := newMetadataLabelQuerier(tx, q).queryLabels()
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
		loaded:     false,
		accessedAt: time.Time{},
		modifyTxn:  0,
	}
}

func (p *indexPartition) unload() {
	p.loaded = false
	clear(p.shards)
}

func (p *indexPartition) load(tx *bbolt.Tx, store Store, tenant string) error {
	if p.loaded {
		return nil
	}
	if tx.ID() <= p.modifyTxn {
		// That would mean that we try to load partitions that have been
		// modified and unloaded after the current transaction has begun:
		// an inevitable invalidation of the in-memory state.
		//
		// This is a precaution against loading partitions that have been
		// modified. In practice this is an extremely rare situation that
		// may occur if the same partition is being loaded and unloaded
		// constantly due to a wrong configuration.
		//
		// The operation must be retried with a new transaction.
		return ErrReadAborted
	}
	for shard := range p.partition.TenantShards[tenant] {
		s, err := store.LoadTenantShard(tx, p.partition.Key, tenant, shard)
		if err != nil {
			return err
		}
		if s != nil && len(s.Blocks) > 0 {
			p.shards[shard] = newIndexShard(s)
			s.Blocks = nil
		}
	}
	p.loaded = true
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.blocks[md.Id]; ok {
		return ErrBlockExists
	}
	s.blocks[md.Id] = md
	return x.StoreBlock(tx, s.TenantShard, md)
}

func (s *indexShard) delete(blocks ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range blocks {
		delete(s.blocks, b)
	}
}

func (s *indexShard) getBlocks(dst []*metastorev1.BlockMeta, blocks ...string) []*metastorev1.BlockMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, b := range blocks {
		md, ok := s.blocks[b]
		if !ok {
			continue
		}
		mdCopy := md.CloneVT()
		s.TenantShard.StringTable.Export(mdCopy)
		dst = append(dst, mdCopy)
	}
	return dst
}
