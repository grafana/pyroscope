package index

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type Index struct {
	Config *Config

	partitionMu      sync.Mutex
	loadedPartitions *lru.Cache[PartitionKey, *indexPartition]
	allPartitions    []*PartitionMeta

	store  Store
	logger log.Logger
}

type Config struct {
	PartitionDuration     time.Duration `yaml:"partition_duration"`
	PartitionCacheSize    int           `yaml:"partition_cache_size"`
	QueryLookaroundPeriod time.Duration `yaml:"query_lookaround_period"`
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.PartitionDuration, prefix+"partition-duration", DefaultConfig.PartitionDuration, "")
	f.IntVar(&cfg.PartitionCacheSize, prefix+"partition-cache-size", DefaultConfig.PartitionCacheSize, "How many partitions to keep loaded in memory.")
	f.DurationVar(&cfg.QueryLookaroundPeriod, prefix+"query-lookaround-period", DefaultConfig.QueryLookaroundPeriod, "")
}

var DefaultConfig = Config{
	PartitionDuration:     24 * time.Hour,
	PartitionCacheSize:    7,
	QueryLookaroundPeriod: time.Hour,
}

type indexPartition struct {
	meta     *PartitionMeta
	loadedAt time.Time
	shards   map[uint32]*indexShard
}

type indexShard struct {
	tenants map[string]*indexTenant
}

type indexTenant struct {
	blocks map[string]*metastorev1.BlockMeta
}

type BlockWithPartition struct {
	Meta  *PartitionMeta
	Block *metastorev1.BlockMeta
}

type Store interface {
	ListPartitions() []PartitionKey
	ReadPartitionMeta(p PartitionKey) (*PartitionMeta, error)

	ListShards(p PartitionKey) []uint32
	ListTenants(p PartitionKey, shard uint32) []string
	ListBlocks(p PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta
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
func NewIndex(store Store, logger log.Logger, cfg *Config) *Index {

	// A fixed cache size gives us bounded memory footprint, however changes to the partition duration could reduce
	// the cache effectiveness.
	// TODO (aleks-p):
	//  - resize the cache at runtime when the config changes
	//  - consider auto-calculating the cache size to ensure we hold data for e.g., the last 24 hours
	cache, _ := lru.New[PartitionKey, *indexPartition](cfg.PartitionCacheSize)

	return &Index{
		loadedPartitions: cache,
		allPartitions:    make([]*PartitionMeta, 0),
		store:            store,
		logger:           logger,
		Config:           cfg,
	}
}

// LoadPartitions reads all partitions from the backing store and loads the recent ones in memory.
func (i *Index) LoadPartitions() {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	i.allPartitions = make([]*PartitionMeta, 0)
	for _, key := range i.store.ListPartitions() {
		pMeta, err := i.store.ReadPartitionMeta(key)
		if err != nil {
			level.Error(i.logger).Log("msg", "error reading partition metadata", "key", key, "err", err)
			continue
		}
		i.allPartitions = append(i.allPartitions, pMeta)
	}

	i.sortPartitions()
}

// ForEachPartition executes the given function concurrently for each partition. It will be called for all partitions,
// regardless if they are fully loaded in memory or not.
func (i *Index) ForEachPartition(ctx context.Context, fn func(meta *PartitionMeta) error) error {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	g, ctx := errgroup.WithContext(ctx)
	for _, meta := range i.allPartitions {
		g.Go(func() error {
			return fn(meta)
		})
	}
	err := g.Wait()
	if err != nil {
		level.Error(i.logger).Log("msg", "error during partition iteration", "err", err)
		return err
	}
	return nil
}

func (i *Index) getPartition(meta *PartitionMeta) *indexPartition {
	p, ok := i.loadedPartitions.Get(meta.Key)
	if !ok {
		level.Info(i.logger).Log("msg", "loading partition", "key", meta.Key)
		p = &indexPartition{
			meta:     meta,
			shards:   make(map[uint32]*indexShard),
			loadedAt: time.Now(),
		}
		i.loadedPartitions.Add(meta.Key, p)
		for _, s := range i.store.ListShards(meta.Key) {
			sh := &indexShard{
				tenants: make(map[string]*indexTenant),
			}
			p.shards[s] = sh

			for _, t := range i.store.ListTenants(meta.Key, s) {
				te := &indexTenant{
					blocks: make(map[string]*metastorev1.BlockMeta),
				}
				for _, b := range i.store.ListBlocks(meta.Key, s, t) {
					te.blocks[b.Id] = b
				}
				sh.tenants[t] = te
			}
		}
	}

	return p
}

// CreatePartitionKey creates a partition key for a block. It is meant to be used for newly inserted blocks, as it relies
// on the index's currently configured partition duration to create the key.
//
// Note: Using this for existing blocks following a partition duration change can produce the wrong key. Callers should
// verify that the returned partition actually contains the block.
func (i *Index) CreatePartitionKey(blockId string) PartitionKey {
	t := ulid.Time(ulid.MustParse(blockId).Time()).UTC()

	var b strings.Builder
	b.Grow(16)

	year, month, day := t.Date()
	b.WriteString(fmt.Sprintf("%04d%02d%02d", year, month, day))

	partitionDuration := i.Config.PartitionDuration
	if partitionDuration < 24*time.Hour {
		hour := (t.Hour() / int(partitionDuration.Hours())) * int(partitionDuration.Hours())
		b.WriteString(fmt.Sprintf("T%02d", hour))
	}

	mDuration := model.Duration(partitionDuration)
	b.WriteString(".")
	b.WriteString(mDuration.String())

	return PartitionKey(b.String())
}

// findPartitionMeta retrieves the partition meta for the given key.
func (i *Index) findPartitionMeta(key PartitionKey) *PartitionMeta {
	loaded, ok := i.loadedPartitions.Get(key)
	if ok {
		return loaded.meta
	}
	for _, p := range i.allPartitions {
		if p.Key == key {
			return p
		}
	}
	return nil
}

// InsertBlock is the primary way for adding blocks to the index.
func (i *Index) InsertBlock(b *metastorev1.BlockMeta) {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	i.insertBlock(b)
}

// insertBlock is the underlying implementation for inserting blocks. It is the caller's responsibility to enforce safe
// concurrent access. The method will create a new partition if needed.
func (i *Index) insertBlock(b *metastorev1.BlockMeta) {
	meta := i.getOrCreatePartitionMeta(b)
	p := i.getPartition(meta)

	s, ok := p.shards[b.Shard]
	if !ok {
		s = &indexShard{
			tenants: make(map[string]*indexTenant),
		}
		p.shards[b.Shard] = s
	}

	ten, ok := s.tenants[b.TenantId]
	if !ok {
		ten = &indexTenant{
			blocks: make(map[string]*metastorev1.BlockMeta),
		}
		s.tenants[b.TenantId] = ten
	}

	ten.blocks[b.Id] = b
}

// GetOrCreatePartitionMeta creates the mapping between blocks and partitions. It may assign the block to an existing
// partition or create a new partition altogether. Meant to be used only in the context of new blocks.
func (i *Index) GetOrCreatePartitionMeta(b *metastorev1.BlockMeta) *PartitionMeta {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()
	return i.getOrCreatePartitionMeta(b)
}

func (i *Index) getOrCreatePartitionMeta(b *metastorev1.BlockMeta) *PartitionMeta {
	key := i.CreatePartitionKey(b.Id)
	meta := i.findPartitionMeta(key)

	if meta == nil {
		ts, duration, _ := key.Parse()
		meta = &PartitionMeta{
			Key:       key,
			Ts:        ts,
			Duration:  duration,
			Tenants:   make([]string, 0),
			tenantMap: make(map[string]struct{}),
		}
		i.allPartitions = append(i.allPartitions, meta)
		i.sortPartitions()
	}

	if b.TenantId != "" {
		meta.AddTenant(b.TenantId)
	} else {
		for _, ds := range b.Datasets {
			meta.AddTenant(ds.TenantId)
		}
	}

	return meta
}

// FindBlock tries to retrieve an existing block from the index. It will load the corresponding partition if it is not
// already loaded. Returns nil if the block cannot be found.
func (i *Index) FindBlock(shardNum uint32, tenant string, blockId string) *metastorev1.BlockMeta {
	// first try the currently mapped partition
	key := i.CreatePartitionKey(blockId)
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	b := i.findBlockInPartition(key, shardNum, tenant, blockId)
	if b != nil {
		return b
	}

	// try other partitions that could contain the block
	t := ulid.Time(ulid.MustParse(blockId).Time()).UTC().UnixMilli()
	for _, p := range i.allPartitions {
		if p.contains(t) {
			b := i.findBlockInPartition(p.Key, shardNum, tenant, blockId)
			if b != nil {
				return b
			}
		}
	}
	return nil
}

func (i *Index) findBlockInPartition(key PartitionKey, shard uint32, tenant string, blockId string) *metastorev1.BlockMeta {
	meta := i.findPartitionMeta(key)
	if meta == nil {
		return nil
	}

	p := i.getPartition(meta)

	s, _ := p.shards[shard]
	if s == nil {
		return nil
	}

	t, _ := s.tenants[tenant]
	if t == nil {
		return nil
	}

	b, _ := t.blocks[blockId]

	return b
}

// FindBlocksInRange retrieves all blocks that might contain data for the given time range and tenants.
//
// It is not enough to scan for partition keys that fall in the given time interval. Partitions are built on top of
// block identifiers which refer to the moment a block was created and not to the timestamps of the profiles contained
// within the block (min_time, max_time). This method works around this by including blocks from adjacent partitions.
//
// FIXME aleks-p: A large query could cause a large number of partitions to be loaded into memory
//   - consider loading partitions in parallel
//   - consider capping the number of loaded partitions
func (i *Index) FindBlocksInRange(start, end int64, tenants map[string]struct{}) ([]*metastorev1.BlockMeta, error) {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()
	startWithLookaround := start - i.Config.QueryLookaroundPeriod.Milliseconds()
	endWithLookaround := end + i.Config.QueryLookaroundPeriod.Milliseconds()

	blocks := make([]*metastorev1.BlockMeta, 0)

	for _, meta := range i.allPartitions { // TODO aleks-p: consider using binary search to find a good starting point
		if meta.overlaps(startWithLookaround, endWithLookaround) {
			p := i.getPartition(meta)
			tenantBlocks := i.collectTenantBlocks(p, start, end, tenants)
			blocks = append(blocks, tenantBlocks...)
		}
	}

	return blocks, nil
}

func (i *Index) sortPartitions() {
	slices.SortFunc(i.allPartitions, func(a, b *PartitionMeta) int {
		return a.compare(b)
	})
}

func (i *Index) collectTenantBlocks(p *indexPartition, start, end int64, tenants map[string]struct{}) []*metastorev1.BlockMeta {
	blocks := make([]*metastorev1.BlockMeta, 0)
	for _, s := range p.shards {
		for tKey, t := range s.tenants {
			_, ok := tenants[tKey]
			if !ok && tKey != "" {
				continue
			}
			for _, block := range t.blocks {
				if start < block.MaxTime && end >= block.MinTime {
					blocks = append(blocks, block)
				}
			}
		}
	}
	return blocks
}

// ReplaceBlocks removes source blocks from the index and inserts replacement blocks into the index. The intended usage
// is for block compaction. The replacement blocks could be added to the same or a different partition.
func (i *Index) ReplaceBlocks(sources []string, sourceShard uint32, sourceTenant string, replacements []*metastorev1.BlockMeta) map[string]*BlockWithPartition {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	for _, newBlock := range replacements {
		i.insertBlock(newBlock)
	}

	deletedBlocks := make(map[string]*BlockWithPartition, len(sources))
	for _, sourceBlock := range sources {
		b, meta := i.deleteBlock(sourceShard, sourceTenant, sourceBlock)
		if b != nil && meta != nil {
			deletedBlocks[sourceBlock] = &BlockWithPartition{
				Meta:  meta,
				Block: b,
			}
		}
	}

	return deletedBlocks
}

// deleteBlock deletes a block from the index. It is the caller's responsibility to enforce safe concurrent access.
func (i *Index) deleteBlock(shard uint32, tenant string, blockId string) (*metastorev1.BlockMeta, *PartitionMeta) {
	// first try the currently mapped partition
	key := i.CreatePartitionKey(blockId)
	if b, meta, ok := i.tryDelete(key, shard, tenant, blockId); ok {
		return b, meta
	}

	// now try all other possible partitions
	t := ulid.Time(ulid.MustParse(blockId).Time()).UTC().UnixMilli()

	for _, p := range i.allPartitions {
		if p.contains(t) {
			if b, meta, ok := i.tryDelete(p.Key, shard, tenant, blockId); ok {
				return b, meta
			}
		}
	}

	return nil, nil
}

func (i *Index) tryDelete(key PartitionKey, shard uint32, tenant string, blockId string) (*metastorev1.BlockMeta, *PartitionMeta, bool) {
	meta := i.findPartitionMeta(key)
	if meta == nil {
		return nil, nil, false
	}

	p := i.getPartition(meta)

	s, ok := p.shards[shard]
	if !ok {
		return nil, nil, false
	}

	t, ok := s.tenants[tenant]
	if !ok {
		return nil, nil, false
	}

	if t.blocks[blockId] != nil {
		b := t.blocks[blockId]
		delete(t.blocks, blockId)
		return b, meta, true
	}

	return nil, nil, false
}
