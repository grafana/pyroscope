package index

import (
	"container/list"
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
	// Maximum number of shards to keep in memory
	defaultMaxShards = 50000
)

var (
	ErrBlockExists = fmt.Errorf("block already exists")
	ErrReadAborted = fmt.Errorf("read aborted")
)

var DefaultConfig = Config{
	PartitionDuration:     partitionDuration,
	QueryLookaroundPeriod: partitionDuration,
	CacheSize:             defaultMaxShards,
}

type Config struct {
	PartitionDuration     time.Duration
	QueryLookaroundPeriod time.Duration `yaml:"query_lookaround_period"`
	CacheSize             int           `yaml:"index_cache_shards"` // Maximum number of shards to keep in memory
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	// FIXME(kolesnikovae): This parameter is not fully supported.
	//  Overlapping partitions are difficult to handle correctly;
	//  without an interval tree, it may also be inefficient.
	//  Instead, we should consider immutable partition ranges:
	//  once a partition is created, all the keys targeting the
	//  time range of the partition should be directed to it.
	cfg.PartitionDuration = DefaultConfig.PartitionDuration
	f.DurationVar(&cfg.QueryLookaroundPeriod, prefix+"query-lookaround-period", DefaultConfig.QueryLookaroundPeriod, "")
	f.IntVar(&cfg.CacheSize, prefix+"index-cache-size", defaultMaxShards, "Maximum number of shards to keep in memory")
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

	// The global lock protects the entire index structure.
	// Index partition shards have their own locks.
	global     sync.Mutex
	partitions []*store.Partition
	shards     map[shardKey]*indexShard
	// LRU tracking for shard eviction
	lruList *list.List

	// The function reports true if partition cannot be evicted.
	keep func(store.PartitionKey) bool

	// We need to ensure that the replacement is atomic, but it may span
	// multiple partitions and tenants (and, theoretically, shards).
	// The mutex synchronizes queries with replacements; insertions are not
	// affected but require synchronization via the global and shard locks.
	// No queries are allowed during the replacement and vice versa.
	//
	// The lock should be taken before the global lock.
	replace sync.RWMutex
}

type shardKey struct {
	partition store.PartitionKey
	tenant    string
	shard     uint32
}

type indexShard struct {
	index      *Index
	mu         sync.RWMutex
	loaded     bool
	modifyTxn  int
	accessedAt time.Time
	blocks     map[string]*metastorev1.BlockMeta
	lruElem    *list.Element
	*store.TenantShard
}

// NewIndex initializes a new metastore index.
func NewIndex(logger log.Logger, s Store, cfg Config) *Index {
	idx := Index{
		shards:     make(map[shardKey]*indexShard),
		partitions: make([]*store.Partition, 0),
		store:      s,
		logger:     logger,
		config:     cfg,
		lruList:    list.New(),
	}
	idx.keep = idx.shouldKeepPartition
	return &idx
}

func NewStore() *store.IndexStore { return store.NewIndexStore() }

func (i *Index) Init(tx *bbolt.Tx) error { return i.store.CreateBuckets(tx) }

func (i *Index) Restore(tx *bbolt.Tx) (err error) {
	i.global.Lock()
	defer i.global.Unlock()

	i.partitions = nil
	clear(i.shards)

	if i.partitions, err = i.store.ListPartitions(tx); err != nil {
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
		if i.shouldKeepPartition(p.Key) {
			level.Info(i.logger).Log("msg", "loading partition in memory")
			for tenant, shards := range p.TenantShards {
				for shard := range shards {
					s := i.getOrCreateIndexShard(p, tenant, shard)
					if err = s.load(tx); err != nil {
						level.Error(i.logger).Log(
							"msg", "failed to load tenant partition shard",
							"partition", p.Key,
							"tenant", tenant,
							"shard", shard,
							"err", err,
						)
						return err
					}
				}
			}
		}
	}

	level.Info(i.logger).Log("msg", "loaded metastore index partitions", "count", len(i.partitions))
	i.sortPartitions()
	return nil
}

func (i *Index) shouldKeepPartition(k store.PartitionKey) bool {
	now := time.Now()
	low := now.Add(-partitionProtectionWindow)
	high := now.Add(partitionProtectionWindow)
	return k.Overlaps(low, high)
}

func (i *Index) sortPartitions() {
	slices.SortFunc(i.partitions, func(a, b *store.Partition) int {
		return a.Compare(b)
	})
}

func (i *Index) InsertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	i.global.Lock()
	defer i.global.Unlock()
	s := i.getOrCreateIndexShard(i.getOrCreatePartition(b), metadata.Tenant(b), b.Shard)
	return s.update(tx, func(shard *indexShard) error {
		i.unload(tx)
		return shard.insert(tx, b)
	})
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

func (i *Index) getOrCreateIndexShard(p *store.Partition, tenant string, shard uint32) *indexShard {
	k := shardKey{partition: p.Key, tenant: tenant, shard: shard}
	s, ok := i.shards[k]
	if !ok {
		s = i.newIndexShard(&store.TenantShard{
			Partition:   p.Key,
			Tenant:      tenant,
			Shard:       shard,
			StringTable: metadata.NewStringTable(),
		})
		i.shards[k] = s
		p.AddTenantShard(tenant, shard)
	}
	if s.lruElem == nil {
		s.lruElem = i.lruList.PushFront(s)
	} else {
		i.lruList.MoveToFront(s.lruElem)
	}
	return s
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
	var err error
	for k, partitioned := range partitionedLists {
		if metas, err = i.getBlockList(metas, tx, k, partitioned); err != nil {
			return nil, err
		}
	}
	return metas, nil
}

func (i *Index) getBlockList(
	metas []*metastorev1.BlockMeta,
	tx *bbolt.Tx,
	key store.PartitionKey,
	list *metastorev1.BlockList,
) ([]*metastorev1.BlockMeta, error) {
	i.global.Lock()
	defer i.global.Unlock()
	p := i.getPartition(key)
	if p == nil {
		return metas, nil
	}
	if !p.HasIndexShard(list.Tenant, list.Shard) {
		return metas, nil
	}
	s := i.getOrCreateIndexShard(p, list.Tenant, list.Shard)
	if s == nil {
		return metas, nil
	}
	return metas, s.view(tx, func(shard *indexShard) error {
		metas = shard.getBlocks(metas, list.Blocks...)
		return nil
	})
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
		s := i.getOrCreateIndexShard(i.getOrCreatePartition(b), metadata.Tenant(b), b.Shard)
		err := s.update(tx, func(s *indexShard) error {
			return s.insert(tx, b)
		})
		switch {
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
		s := i.getOrCreateIndexShard(p, partitioned.Tenant, partitioned.Shard)
		err := s.update(tx, func(s *indexShard) error {
			s.delete(partitioned.Blocks...)
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (i *Index) unload(tx *bbolt.Tx) {
	if tx.DB().Stats().OpenTxN > 0 {
		// A transaction may be opened right after the check.
		// The reader must ensure that the partition to load has
		// not been modified after the transaction has begun.
		return
	}

	for i.lruList.Len() > i.config.CacheSize {
		elem := i.lruList.Back()
		if elem == nil {
			break
		}

		shard := elem.Value.(*indexShard)
		if i.keep(shard.Partition) {
			// Move to front to skip checking.
			i.lruList.MoveToFront(elem)
			continue
		}

		if shard.loaded {
			level.Debug(i.logger).Log(
				"msg", "evicting shard from memory",
				"partition", shard.Partition,
				"tenant", shard.Tenant,
				"shard", shard.Shard,
			)

			shard.unload(tx)
			i.lruList.Remove(shard.lruElem)
			shard.lruElem = nil
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

func (i *Index) newIndexShard(s *store.TenantShard) *indexShard {
	return &indexShard{
		index:       i,
		blocks:      make(map[string]*metastorev1.BlockMeta),
		lruElem:     nil, // Will be set when added to LRU list
		TenantShard: s,
	}
}

func (s *indexShard) load(tx *bbolt.Tx) error {
	if s.loaded {
		s.accessedAt = time.Now()
		return nil
	}
	if tx.ID() < s.modifyTxn {
		// That would mean that we try to load a shard that has been
		// modified and unloaded after the current transaction has begun:
		// an inevitable invalidation of the in-memory state.
		//
		// This is a precaution against loading data that have been
		// modified. In practice this is an extremely rare situation
		// that may occur if the same shard is being loaded and unloaded
		// constantly due to a wrong configuration.
		//
		// The operation must be retried with a new transaction.
		return ErrReadAborted
	}
	storedShard, err := s.index.store.LoadTenantShard(tx, s.Partition, s.Tenant, s.Shard)
	if err != nil {
		return err
	}
	if storedShard != nil {
		if storedShard.StringTable != nil {
			s.StringTable = storedShard.StringTable
		}
		if len(storedShard.Blocks) > 0 {
			for _, md := range storedShard.Blocks {
				s.blocks[md.Id] = md
			}
			storedShard.Blocks = nil
		}
	}
	s.loaded = true
	s.modifyTxn = tx.ID()
	s.accessedAt = time.Now()
	return nil
}

func (s *indexShard) unload(tx *bbolt.Tx) {
	// As we want to free up memory, we need
	// to release the objects, but keep them
	// valid.
	s.blocks = make(map[string]*metastorev1.BlockMeta)
	s.StringTable = metadata.NewStringTable()
	s.accessedAt = time.Time{}
	s.modifyTxn = tx.ID()
	s.loaded = false
}

func (s *indexShard) insert(tx *bbolt.Tx, md *metastorev1.BlockMeta) error {
	if _, ok := s.blocks[md.Id]; ok {
		return ErrBlockExists
	}
	s.blocks[md.Id] = md
	return s.index.store.StoreBlock(tx, s.TenantShard, md)
}

func (s *indexShard) delete(blocks ...string) {
	for _, b := range blocks {
		delete(s.blocks, b)
	}
}

func (s *indexShard) getBlocks(dst []*metastorev1.BlockMeta, blocks ...string) []*metastorev1.BlockMeta {
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

func (s *indexShard) view(tx *bbolt.Tx, fn func(*indexShard) error) error {
	s.mu.Lock()
	if err := s.load(tx); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return ErrReadAborted
	}
	return fn(s)
}

func (s *indexShard) update(tx *bbolt.Tx, fn func(*indexShard) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(tx); err != nil {
		return err
	}
	return fn(s)
}
