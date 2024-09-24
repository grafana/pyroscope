package index

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

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
//
// Data can be unloaded via StartCleanupLoop() which is meant to be called by the index owner and ran in a goroutine.
func NewIndex(store Store, logger log.Logger, cfg *Config) *Index {
	return &Index{
		loadedPartitions: make(map[PartitionKey]*indexPartition),
		allPartitions:    make([]*PartitionMeta, 0),
		store:            store,
		logger:           logger,
		config:           cfg,
	}
}

// LoadPartitions reads all partitions from the backing store and loads the recent ones in memory.
func (i *Index) LoadPartitions() {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	i.loadedPartitions = make(map[PartitionKey]*indexPartition)
	i.allPartitions = make([]*PartitionMeta, 0)
	for _, key := range i.store.ListPartitions() {
		pMeta, err := i.store.ReadPartitionMeta(key)
		if err != nil {
			level.Error(i.logger).Log("msg", "error reading partition metadata", "key", key, "err", err)
			continue
		}
		i.allPartitions = append(i.allPartitions, pMeta)
		if pMeta.Ts.Add(i.config.PartitionTTL).Before(time.Now()) {
			// too old, will load on demand
			continue
		}
		_, _ = i.getOrLoadPartition(pMeta)
	}

	i.sortPartitions()
}

// ForEachPartition executes the given function concurrently for each partition. It will be called for all partitions,
// regardless if they are fully loaded in memory or not.
func (i *Index) ForEachPartition(ctx context.Context, fn func(meta *PartitionMeta)) {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	g, ctx := errgroup.WithContext(ctx)
	for _, meta := range i.allPartitions {
		g.Go(func() error {
			fn(meta)
			return nil
		})
	}
	err := g.Wait()
	if err != nil {
		level.Error(i.logger).Log("msg", "error during partition iteration", "err", err)
	}
}

func (i *Index) getOrCreatePartition(meta *PartitionMeta) *indexPartition {
	p, ok := i.loadedPartitions[meta.Key]
	if !ok {
		level.Info(i.logger).Log("msg", "creating new partition", "key", meta.Key)
		p = &indexPartition{
			meta:     meta,
			shards:   make(map[uint32]*indexShard),
			loadedAt: time.Now(),
		}
		i.loadedPartitions[meta.Key] = p
	}
	return p
}

func (i *Index) getOrLoadPartition(meta *PartitionMeta) (*indexPartition, error) {
	p, ok := i.loadedPartitions[meta.Key]
	if !ok {
		level.Info(i.logger).Log("msg", "loading partition", "key", meta.Key)
		p = &indexPartition{
			meta:     meta,
			shards:   make(map[uint32]*indexShard),
			loadedAt: time.Now(),
		}
		i.loadedPartitions[meta.Key] = p
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

	return p, nil
}

// GetPartitionKey creates a partition key for a block. It is meant to be used for newly inserted blocks, as it relies
// on the index's currently configured partition duration to create the key.
//
// FIXME aleks-p: Using this for existing blocks following a partition duration change will produce an invalid key.
//   - option 1: create a lookup table (block id -> partition key)
//   - option 2: scan through existing partitions for candidates
func (i *Index) GetPartitionKey(blockId string) PartitionKey {
	t := ulid.Time(ulid.MustParse(blockId).Time()).UTC()

	var b strings.Builder
	b.Grow(16)

	year, month, day := t.Date()
	b.WriteString(fmt.Sprintf("%04d%02d%02d", year, month, day))

	partitionDuration := i.config.PartitionDuration
	if partitionDuration < 24*time.Hour {
		hour := (t.Hour() / int(partitionDuration.Hours())) * int(partitionDuration.Hours())
		b.WriteString(fmt.Sprintf("T%02d", hour))
	}

	mDuration := model.Duration(partitionDuration)
	b.WriteString(".")
	b.WriteString(mDuration.String())

	return PartitionKey(b.String())
}

// FindPartitionMeta retrieves the partition meta for the given key.
func (i *Index) FindPartitionMeta(key PartitionKey) *PartitionMeta {
	loaded, ok := i.loadedPartitions[key]
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
func (i *Index) InsertBlock(b *metastorev1.BlockMeta) error {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	return i.insertBlock(b)
}

// insertBlock is the underlying implementation for inserting blocks. It is the caller's responsibility to enforce safe
// concurrent access. The method will create a new partition if needed.
func (i *Index) insertBlock(b *metastorev1.BlockMeta) error {
	meta, err := i.GetOrCreatePartitionMeta(b)
	if err != nil {
		return err
	}

	p := i.getOrCreatePartition(meta)

	p.shardsMu.Lock()
	defer p.shardsMu.Unlock()

	s, ok := p.shards[b.Shard]
	if !ok {
		s = &indexShard{
			tenants: make(map[string]*indexTenant),
		}
		p.shards[b.Shard] = s
	}

	s.tenantsMu.Lock()
	defer s.tenantsMu.Unlock()

	ten, ok := s.tenants[b.TenantId]
	if !ok {
		ten = &indexTenant{
			blocks: make(map[string]*metastorev1.BlockMeta),
		}
		s.tenants[b.TenantId] = ten
	}

	ten.blocksMu.Lock()
	defer ten.blocksMu.Unlock()

	ten.blocks[b.Id] = b
	return nil
}

// GetOrCreatePartitionMeta makes the mapping between blocks and partitions. It may assign the block to an existing
// partition or create a new partition altogether. Meant to be used only in the context of new blocks.
func (i *Index) GetOrCreatePartitionMeta(b *metastorev1.BlockMeta) (*PartitionMeta, error) {
	key := i.GetPartitionKey(b.Id)
	meta := i.FindPartitionMeta(key)

	if meta == nil {
		ts, duration, err := key.Parse()
		if err != nil {
			return nil, err
		}
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

	return meta, nil
}

// FindBlock tries to retrieve an existing block from the index. It will load the corresponding partition if it is not
// already loaded. Returns nil if the block cannot be found.
func (i *Index) FindBlock(shardNum uint32, tenant string, id string) *metastorev1.BlockMeta {
	key := i.GetPartitionKey(id)

	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	meta := i.FindPartitionMeta(key)
	if meta == nil {
		return nil
	}

	p, err := i.getOrLoadPartition(meta)
	if err != nil {
		return nil
	}

	p.shardsMu.Lock()
	defer p.shardsMu.Unlock()
	s, _ := p.shards[shardNum]
	if s == nil {
		return nil
	}

	s.tenantsMu.Lock()
	defer s.tenantsMu.Unlock()
	t, _ := s.tenants[tenant]
	if t == nil {
		return nil
	}

	t.blocksMu.Lock()
	defer t.blocksMu.Unlock()
	b, _ := t.blocks[id]

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

	blocks := make([]*metastorev1.BlockMeta, 0)

	firstPartitionIdx, lastPartitionIdx := -1, -1
	for idx, meta := range i.allPartitions {
		if meta.Key.inRange(start, end) {
			if firstPartitionIdx == -1 {
				firstPartitionIdx = idx
			}
			p, err := i.getOrLoadPartition(meta)
			if err != nil {
				level.Error(i.logger).Log("msg", "error loading partition", "key", meta.Key, "err", err)
				return nil, err
			}
			tenantBlocks := i.collectTenantBlocks(p, tenants)
			blocks = append(blocks, tenantBlocks...)
		} else if firstPartitionIdx != -1 {
			lastPartitionIdx = idx - 1
		}
	}

	if firstPartitionIdx > 0 {
		meta := i.allPartitions[firstPartitionIdx-1]
		p, err := i.getOrLoadPartition(meta)
		if err != nil {
			level.Error(i.logger).Log("msg", "error loading previous partition", "key", meta.Key, "err", err)
			return nil, err
		}
		tenantBlocks := i.collectTenantBlocks(p, tenants)
		blocks = append(blocks, tenantBlocks...)
	}

	if lastPartitionIdx > -1 && lastPartitionIdx < len(i.allPartitions)-1 {
		meta := i.allPartitions[lastPartitionIdx+1]
		p, err := i.getOrLoadPartition(meta)
		if err != nil {
			level.Error(i.logger).Log("msg", "error loading next partition", "key", meta.Key, "err", err)
			return nil, err
		}
		tenantBlocks := i.collectTenantBlocks(p, tenants)
		blocks = append(blocks, tenantBlocks...)
	}

	return blocks, nil
}

func (i *Index) sortPartitions() {
	slices.SortFunc(i.allPartitions, func(a, b *PartitionMeta) int {
		return a.Key.compare(b.Key)
	})
}

func (i *Index) collectTenantBlocks(p *indexPartition, tenants map[string]struct{}) []*metastorev1.BlockMeta {
	p.shardsMu.Lock()
	defer p.shardsMu.Unlock()
	blocks := make([]*metastorev1.BlockMeta, 0)
	for _, s := range p.shards {
		s.tenantsMu.Lock()
		for tKey, t := range s.tenants {
			_, ok := tenants[tKey]
			if !ok && tKey != "" {
				continue
			}
			t.blocksMu.Lock()
			for _, block := range t.blocks {
				blocks = append(blocks, block)
			}
			t.blocksMu.Unlock()
		}
		s.tenantsMu.Unlock()
	}
	return blocks
}

// ReplaceBlocks removes source blocks from the index and inserts replacement blocks into the index. The intended usage
// is for block compaction. The replacement blocks could be added to the same or a different partition.
func (i *Index) ReplaceBlocks(sources []string, sourceShard uint32, sourceTenant string, replacements []*metastorev1.BlockMeta) error {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	for _, newBlock := range replacements {
		err := i.insertBlock(newBlock)
		if err != nil {
			return err
		}
	}

	for _, sourceBlock := range sources {
		i.deleteBlock(sourceShard, sourceTenant, sourceBlock)
	}

	return nil
}

// deleteBlock deletes a block from the index. It is the caller's responsibility to enforce safe concurrent access.
func (i *Index) deleteBlock(shard uint32, tenant string, blockId string) {
	key := i.GetPartitionKey(blockId)

	p, ok := i.loadedPartitions[key]
	if !ok {
		return
	}

	p.shardsMu.Lock()
	defer p.shardsMu.Unlock()

	s, ok := p.shards[shard]
	if !ok {
		return
	}

	s.tenantsMu.Lock()
	defer s.tenantsMu.Unlock()

	t, ok := s.tenants[tenant]
	if !ok {
		return
	}

	t.blocksMu.Lock()
	defer t.blocksMu.Unlock()

	delete(t.blocks, blockId)
}

// StartCleanupLoop unloads partitions from memory at an interval.
func (i *Index) StartCleanupLoop(ctx context.Context) {
	t := time.NewTicker(i.config.CleanupInterval)
	defer func() {
		t.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.unloadPartitions(time.Now().Add(-i.config.PartitionTTL))
			// TODO aleks-p: Physically delete all partitions older than 30 days
		}
	}
}

// unloadPartitions removes all loaded partitions that were loaded before the given threshold.
func (i *Index) unloadPartitions(unloadThreshold time.Time) {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	level.Info(i.logger).Log("msg", "unloading partitions", "threshold", unloadThreshold)
	for k, p := range i.loadedPartitions {
		if p.loadedAt.Before(unloadThreshold) {
			level.Info(i.logger).Log("msg", "unloading partition", "key", k, "loaded_at", p.loadedAt)
			delete(i.loadedPartitions, k)
		}
	}
}
