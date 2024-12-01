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
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

var ErrBlockExists = fmt.Errorf("block already exists")

type Store interface {
	CreateBuckets(*bbolt.Tx) error
	StoreBlock(tx *bbolt.Tx, p store.PartitionKey, shard uint32, tenant string, id string, md *metastorev1.BlockMeta) error
	StoreStrings(tx *bbolt.Tx, p store.PartitionKey, shard uint32, tenant string, offset int, strings []string) error
	LoadStrings(tx *bbolt.Tx, p store.PartitionKey, shard uint32, tenant string) iter.Iterator[string]
	DeleteBlockList(*bbolt.Tx, store.PartitionKey, *metastorev1.BlockList) error

	ListPartitions(*bbolt.Tx) []store.PartitionKey
	ListShards(*bbolt.Tx, store.PartitionKey) []uint32
	ListTenants(tx *bbolt.Tx, p store.PartitionKey, shard uint32) []string
	ListBlocks(tx *bbolt.Tx, p store.PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta
}

type Index struct {
	config *Config

	partitionMu      sync.Mutex
	loadedPartitions map[cacheKey]*indexPartition
	partitions       []*PartitionMeta

	store  Store
	logger log.Logger
}

type Config struct {
	// FIXME(kolesnikovae): This parameter is not fully supported.
	PartitionDuration     time.Duration `yaml:"partition_duration" doc:"hidden"`
	PartitionCacheSize    int           `yaml:"partition_cache_size"`
	QueryLookaroundPeriod time.Duration `yaml:"query_lookaround_period"`
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.PartitionDuration, prefix+"partition-duration", DefaultConfig.PartitionDuration, "")
	f.IntVar(&cfg.PartitionCacheSize, prefix+"partition-cache-size", DefaultConfig.PartitionCacheSize, "How many partitions to keep loaded in memory.")
	f.DurationVar(&cfg.QueryLookaroundPeriod, prefix+"query-lookaround-period", DefaultConfig.QueryLookaroundPeriod, "")
}

const (
	// Indicates that partitions within this window are "protected" from being unloaded.
	partitionProtectionWindow = 30 * time.Minute
	partitionDuration         = 6 * time.Hour
)

var DefaultConfig = Config{
	PartitionDuration:     partitionDuration,
	QueryLookaroundPeriod: partitionDuration,
	PartitionCacheSize:    7,
}

type indexPartition struct {
	meta       *PartitionMeta
	accessedAt time.Time
	shards     map[uint32]*indexShard
}

type indexShard struct {
	tenant  string
	shard   uint32
	blocks  map[string]*metastorev1.BlockMeta
	strings *block.MetadataStrings
}

type cacheKey struct {
	partitionKey store.PartitionKey
	tenant       string
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
func NewIndex(logger log.Logger, store Store, cfg *Config) *Index {
	// A fixed cache size gives us bounded memory footprint, however changes to the partition duration could reduce
	// the cache effectiveness.
	// TODO (aleks-p):
	//  - resize the cache at runtime when the config changes
	//  - consider auto-calculating the cache size to ensure we hold data for e.g., the last 24 hours
	return &Index{
		loadedPartitions: make(map[cacheKey]*indexPartition, cfg.PartitionCacheSize),
		partitions:       make([]*PartitionMeta, 0),
		store:            store,
		logger:           logger,
		config:           cfg,
	}
}

func NewStore() *store.IndexStore {
	return store.NewIndexStore()
}

// LoadPartitions reads all partitions from the backing store and loads the recent ones in memory.
func (i *Index) LoadPartitions(tx *bbolt.Tx) {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	i.partitions = i.partitions[:0]
	clear(i.loadedPartitions)
	for _, key := range i.store.ListPartitions(tx) {
		p := i.loadPartitionMeta(tx, key)
		level.Info(i.logger).Log(
			"msg", "loaded metastore index partition",
			"key", key,
			"ts", p.Timestamp.Format(time.RFC3339),
			"duration", p.Duration,
			"tenants", strings.Join(p.Tenants, ","))
		i.partitions = append(i.partitions, p)

		// load the currently active partition
		if p.contains(time.Now()) {
			i.loadEntirePartition(tx, p)
		}
	}
	level.Info(i.logger).Log("msg", "loaded metastore index partitions", "count", len(i.partitions))

	i.sortPartitions()
}

func (i *Index) loadPartitionMeta(tx *bbolt.Tx, key store.PartitionKey) *PartitionMeta {
	timestamp, duration, _ := key.Parse()
	p := &PartitionMeta{
		Key:       key,
		Timestamp: timestamp,
		Duration:  duration,
		Tenants:   make([]string, 0),
		tenantMap: make(map[string]struct{}),
	}
	for _, s := range i.store.ListShards(tx, key) {
		for _, t := range i.store.ListTenants(tx, key, s) {
			p.AddTenant(t)
		}
	}
	return p
}

func (i *Index) loadEntirePartition(tx *bbolt.Tx, p *PartitionMeta) {
	for _, s := range i.store.ListShards(tx, p.Key) {
		for _, t := range i.store.ListTenants(tx, p.Key, s) {
			k := cacheKey{partitionKey: p.Key, tenant: t}
			partition, ok := i.loadedPartitions[k]
			if !ok {
				partition = newIndexPartition(p)
				i.loadedPartitions[k] = partition
			}
			shard, ok := partition.shards[s]
			if !ok {
				shard = newIndexShard(s, t)
				partition.shards[s] = shard
			}
			shard.load(tx, i.store, p, s, t)
		}
	}
}

func (i *Index) getOrCreatePartition(tx *bbolt.Tx, p *PartitionMeta, tenant string) *indexPartition {
	k := cacheKey{partitionKey: p.Key, tenant: tenant}
	partition, ok := i.loadedPartitions[k]
	if !ok {
		partition = newIndexPartition(p)
		partition.load(tx, i.store, tenant)
		i.loadedPartitions[k] = partition
	}
	partition.accessedAt = time.Now()
	i.unloadPartitions()
	return partition
}

func (i *Index) getOrLoadPartition(tx *bbolt.Tx, p *PartitionMeta, tenant string) *indexPartition {
	if !p.HasTenant(tenant) {
		return nil
	}
	k := cacheKey{partitionKey: p.Key, tenant: tenant}
	partition, ok := i.loadedPartitions[k]
	if ok {
		return partition
	}
	partition = newIndexPartition(p)
	if !partition.load(tx, i.store, tenant) {
		// Just a safety check to avoid displacing
		// partitions that are still in use.
		return nil
	}
	partition.accessedAt = time.Now()
	i.loadedPartitions[k] = partition
	i.unloadPartitions()
	return partition
}

// findPartitionMeta retrieves the partition meta for the given key.
func (i *Index) findPartitionMeta(key store.PartitionKey) *PartitionMeta {
	for _, p := range i.partitions {
		if p.Key == key {
			return p
		}
	}
	return nil
}

func (i *Index) InsertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()
	if i.findBlock(tx, b.Shard, block.Tenant(b), block.ID(b)) {
		return ErrBlockExists
	}
	return i.insertBlock(tx, b)
}

// insertBlock is the underlying implementation for inserting blocks. It is the caller's responsibility to enforce safe
// concurrent access. The method will create a new partition if needed.
func (i *Index) insertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	p := i.getOrCreatePartitionMeta(b)
	partition := i.getOrCreatePartition(tx, p, block.Tenant(b))
	shard, ok := partition.shards[b.Shard]
	if !ok {
		shard = newIndexShard(b.Shard, block.Tenant(b))
		partition.shards[b.Shard] = shard
	}
	return shard.insert(tx, i.store, partition.meta, b)
}

func (i *Index) getOrCreatePartitionMeta(b *metastorev1.BlockMeta) *PartitionMeta {
	key := store.CreatePartitionKey(block.ID(b), i.config.PartitionDuration)
	p := i.findPartitionMeta(key)
	if p == nil {
		timestamp, duration, _ := key.Parse()
		p = &PartitionMeta{
			Key:       key,
			Timestamp: timestamp,
			Duration:  duration,
			Tenants:   make([]string, 0),
			tenantMap: make(map[string]struct{}),
		}
		i.partitions = append(i.partitions, p)
		i.sortPartitions()
	}
	if block.Tenant(b) != "" {
		p.AddTenant(block.Tenant(b))
	} else {
		for _, ds := range b.Datasets {
			p.AddTenant(b.StringTable[ds.Tenant])
		}
	}
	return p
}

func (i *Index) FindBlocks(tx *bbolt.Tx, list *metastorev1.BlockList) []*metastorev1.BlockMeta {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	partitions := make(map[store.PartitionKey]struct{})
	left := make(map[string]struct{})
	for _, id := range list.Blocks {
		partitions[store.CreatePartitionKey(id, i.config.PartitionDuration)] = struct{}{}
		left[id] = struct{}{}
	}

	found := make([]*metastorev1.BlockMeta, 0, len(list.Blocks))
	for key := range partitions {
		p := i.findPartitionMeta(key)
		if p == nil {
			continue
		}
		partition := i.getOrLoadPartition(tx, p, list.Tenant)
		if partition == nil {
			continue
		}
		shard, _ := partition.shards[list.Shard]
		if shard == nil {
			continue
		}
		for b := range left {
			if md := shard.get(b); md != nil {
				found = append(found, md)
				delete(left, b)
			}
		}
	}

	return found
}

func (i *Index) findBlock(tx *bbolt.Tx, shard uint32, tenant string, blockID string) bool {
	key := store.CreatePartitionKey(blockID, i.config.PartitionDuration)
	p := i.findPartitionMeta(key)
	if p == nil {
		return false
	}

	partition := i.getOrLoadPartition(tx, p, tenant)
	if partition == nil {
		return false
	}
	s := partition.shards[shard]
	if s == nil {
		return false
	}

	_, ok := s.blocks[blockID]
	return ok
}

func (i *Index) sortPartitions() {
	slices.SortFunc(i.partitions, func(a, b *PartitionMeta) int {
		return a.compare(b)
	})
}

// ReplaceBlocks removes source blocks from the index and inserts replacement blocks into the index. The intended usage
// is for block compaction. The replacement blocks could be added to the same or a different partition.
func (i *Index) ReplaceBlocks(tx *bbolt.Tx, compacted *metastorev1.CompactedBlocks) error {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()
	for _, b := range compacted.NewBlocks {
		if err := i.insertBlock(tx, b); err != nil {
			return err
		}
	}
	return i.deleteBlockList(tx, compacted.SourceBlocks)
}

func (i *Index) deleteBlockList(tx *bbolt.Tx, list *metastorev1.BlockList) error {
	partitions := make(map[store.PartitionKey]*metastorev1.BlockList)
	for _, b := range list.Blocks {
		k := store.CreatePartitionKey(b, i.config.PartitionDuration)
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
	for k, partitioned := range partitions {
		if err := i.store.DeleteBlockList(tx, k, partitioned); err != nil {
			return err
		}
		ck := cacheKey{partitionKey: k, tenant: list.Tenant}
		partition := i.loadedPartitions[ck]
		if partition == nil {
			continue
		}
		shard := partition.shards[partitioned.Shard]
		if shard == nil {
			continue
		}
		for _, b := range partitioned.Blocks {
			delete(shard.blocks, b)
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
			if p.meta.overlaps(time.Now().Add(-partitionProtectionWindow), time.Now().Add(partitionProtectionWindow)) {
				continue
			}
			level.Debug(i.logger).Log("unloading metastore index partition", "key", p.meta.Key, "accessed_at", p.accessedAt.Format(time.RFC3339))
			cKey := cacheKey{
				partitionKey: p.meta.Key,
				tenant:       t,
			}
			delete(i.loadedPartitions, cKey)
			toRemove--
			if toRemove == 0 {
				break
			}
		}
	}
}

func (i *Index) Init(tx *bbolt.Tx) error {
	return i.store.CreateBuckets(tx)
}

func (i *Index) Restore(tx *bbolt.Tx) error {
	i.LoadPartitions(tx)
	return nil
}

func (i *Index) GetTenantStats(tenant string) *metastorev1.TenantStats {
	stats := &metastorev1.TenantStats{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}

	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

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

	return stats
}

func (i *Index) QueryMetadata(tx *bbolt.Tx, query MetadataQuery) iter.Iterator[*metastorev1.BlockMeta] {
	q, err := newMetadataQuery(i, query)
	if err != nil {
		return iter.NewErrIterator[*metastorev1.BlockMeta](err)
	}
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()
	// TODO(kolesnikovae): We collect blocks with the mutex held, which
	//  will cause contention and latency issues. Fix it once we make
	//  locks more granular (partition-tenant-shard level).
	metas, err := iter.Slice[*metastorev1.BlockMeta](q.iterator(tx))
	if err != nil {
		return iter.NewErrIterator[*metastorev1.BlockMeta](err)
	}
	return iter.NewSliceIterator(metas)
}

func (i *Index) shardIterator(tx *bbolt.Tx, startTime, endTime time.Time, tenants ...string) iter.Iterator[*indexShard] {
	startTime = startTime.Add(-i.config.QueryLookaroundPeriod)
	endTime = endTime.Add(i.config.QueryLookaroundPeriod)
	si := shardIterator{
		tx:         tx,
		partitions: make([]*PartitionMeta, 0, len(i.partitions)),
		tenants:    make([]string, 1, 1+len(tenants)), // +1 For anon tenant.
		index:      i,
	}
	for _, t := range tenants {
		si.tenants = append(si.tenants, t)
	}
	for _, p := range i.partitions {
		if !p.overlaps(startTime, endTime) {
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

func newIndexPartition(p *PartitionMeta) *indexPartition {
	return &indexPartition{
		meta:   p,
		shards: make(map[uint32]*indexShard),
	}
}

func (p *indexPartition) load(tx *bbolt.Tx, store Store, tenant string) (loaded bool) {
	for _, s := range store.ListShards(tx, p.meta.Key) {
		shard := newIndexShard(s, tenant)
		if shard.load(tx, store, p.meta, s, tenant) {
			p.shards[s] = shard
			loaded = true
		}
	}
	return loaded
}

func newIndexShard(shard uint32, tenant string) *indexShard {
	return &indexShard{
		tenant:  tenant,
		shard:   shard,
		blocks:  make(map[string]*metastorev1.BlockMeta),
		strings: block.NewMetadataStringTable(),
	}
}

func (s *indexShard) load(tx *bbolt.Tx, store Store, p *PartitionMeta, shard uint32, tenant string) bool {
	stringIter := store.LoadStrings(tx, p.Key, shard, tenant)
	defer func() {
		_ = stringIter.Close()
	}()
	// TODO: Error handling.
	if err := s.strings.Load(stringIter); err != nil {
		return false
	}
	if len(s.strings.Strings) == 0 {
		return false
	}
	for _, b := range store.ListBlocks(tx, p.Key, shard, tenant) {
		s.blocks[s.strings.Strings[b.Id]] = b
	}
	return len(s.blocks) > 0
}

func (s *indexShard) insert(tx *bbolt.Tx, store Store, p *PartitionMeta, md *metastorev1.BlockMeta) error {
	// Keep params as they may be removed at strings.Import.
	blockID := block.ID(md)
	tenant := block.Tenant(md)
	shard := md.Shard
	if _, ok := s.blocks[blockID]; ok {
		return nil
	}
	// Clear the string table to save memory.
	s.strings.Import(md)
	if err := store.StoreStrings(tx, p.Key, shard, tenant, len(s.strings.Strings), md.StringTable); err != nil {
		return err
	}
	s.blocks[blockID] = md
	md.StringTable = nil
	return store.StoreBlock(tx, p.Key, shard, tenant, blockID, md)
}

func (s *indexShard) get(blockID string) *metastorev1.BlockMeta {
	md, ok := s.blocks[blockID]
	if !ok {
		return nil
	}
	mdCopy := md.CloneVT()
	s.strings.Export(mdCopy)
	return mdCopy
}

type shardIterator struct {
	tx         *bbolt.Tx
	index      *Index
	tenants    []string
	partitions []*PartitionMeta
	shards     []*indexShard
	cur        int
}

func (si *shardIterator) Close() error { return nil }

func (si *shardIterator) Err() error { return nil }

func (si *shardIterator) At() *indexShard { return si.shards[si.cur] }

func (si *shardIterator) Next() bool {
	if n := si.cur + 1; n < len(si.shards) {
		si.cur = n
		return true
	}
	si.cur = 0
	si.shards = si.shards[:0]
	for len(si.shards) == 0 && len(si.partitions) > 0 {
		si.loadShards(si.partitions[0])
		si.partitions = si.partitions[1:]
	}
	return si.cur < len(si.shards)
}

func (si *shardIterator) loadShards(p *PartitionMeta) {
	for _, t := range si.tenants {
		partition := si.index.getOrLoadPartition(si.tx, p, t)
		if partition == nil {
			continue
		}
		for _, shard := range partition.shards {
			si.shards = append(si.shards, shard)
		}
	}
	slices.SortFunc(si.shards, compareShards)
	si.shards = slices.Compact(si.shards)
}

func compareShards(a, b *indexShard) int {
	if a.tenant == b.tenant {
		return int(a.shard) - int(b.shard)
	}
	return strings.Compare(a.tenant, b.tenant)
}
