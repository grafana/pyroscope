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

func NewIndex(store Store, logger log.Logger, cfg *Config) *Index {
	return &Index{
		loadedPartitions: make(map[PartitionKey]*indexPartition),
		allPartitions:    make([]*PartitionMeta, 0),
		store:            store,
		logger:           logger,
		config:           cfg,
	}
}

// LoadPartitions reads all partitions from the backing store and loads the most recent ones in memory.
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

func (i *Index) InsertBlock(b *metastorev1.BlockMeta) error {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	return i.insertBlockInternal(b)
}

func (i *Index) insertBlockInternal(b *metastorev1.BlockMeta) error {
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

func (i *Index) ReplaceBlocks(sources []string, sourceShard uint32, sourceTenant string, replacements []*metastorev1.BlockMeta) error {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	for _, newBlock := range replacements {
		err := i.insertBlockInternal(newBlock)
		if err != nil {
			return err
		}
	}

	for _, sourceBlock := range sources {
		i.deleteBlock(sourceShard, sourceTenant, sourceBlock)
	}

	return nil
}

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
		}
	}
}

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
