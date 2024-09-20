package index

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type PartitionKey string

type PartitionMeta struct {
	Key      PartitionKey  `json:"key"`
	Ts       time.Time     `json:"ts"`
	Duration time.Duration `json:"duration"`
	Tenants  []string      `json:"tenants"`

	tenantMap map[string]struct{}
}

func (m *PartitionMeta) HasTenant(tenant string) bool {
	m.loadTenants()
	_, ok := m.tenantMap[tenant]
	return ok
}

func (m *PartitionMeta) StartTime() time.Time {
	return m.Ts
}

func (m *PartitionMeta) EndTime() time.Time {
	return m.Ts.Add(m.Duration)
}

func (m *PartitionMeta) loadTenants() {
	if len(m.Tenants) > 0 && len(m.tenantMap) == 0 {
		m.tenantMap = make(map[string]struct{}, len(m.Tenants))
		for _, t := range m.Tenants {
			m.tenantMap[t] = struct{}{}
		}
	}
}

func (m *PartitionMeta) AddTenant(tenant string) {
	m.loadTenants()
	if _, ok := m.tenantMap[tenant]; !ok {
		m.tenantMap[tenant] = struct{}{}
		m.Tenants = append(m.Tenants, tenant)
	}
}

type Index struct {
	partitionMu      sync.Mutex
	loadedPartitions map[PartitionKey]*fullPartition
	allPartitions    []*PartitionMeta

	store  Store
	logger log.Logger

	partitionDuration time.Duration
}

func NewIndex(store Store, logger log.Logger) *Index {
	return &Index{
		loadedPartitions:  make(map[PartitionKey]*fullPartition),
		allPartitions:     make([]*PartitionMeta, 0),
		store:             store,
		logger:            logger,
		partitionDuration: time.Hour,
	}
}

type Store interface {
	ListPartitions() []PartitionKey
	ReadPartitionMeta(p PartitionKey) (*PartitionMeta, error)

	ListShards(p PartitionKey) []uint32
	ListTenants(p PartitionKey, shard uint32) []string
	ListBlocks(p PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta

	LoadBlock(p PartitionKey, shard uint32, tenant string, blockId string) *metastorev1.BlockMeta
}

const (
	dayLayout  = "20060102"
	hourLayout = "20060102T15"
)

func getTimeLayout(d time.Duration) string {
	if d >= 24*time.Hour {
		return dayLayout
	} else {
		return hourLayout
	}
}

func (k PartitionKey) parse() (t time.Time, d time.Duration, err error) {
	parts := strings.Split(string(k), ".")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid partition key: %s", k)
	}
	d, err = time.ParseDuration(parts[1])
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid duration in partition key: %s", k)
	}
	t, err = time.Parse(getTimeLayout(d), parts[0])
	return t, d, err
}

func (k PartitionKey) compare(other PartitionKey) int {
	if k == other {
		return 0
	}
	tSelf, _, err := k.parse()
	if err != nil {
		return strings.Compare(string(k), string(other))
	}
	tOther, _, err := other.parse()
	if err != nil {
		return strings.Compare(string(k), string(other))
	}
	return tSelf.Compare(tOther)
}

func (k PartitionKey) inRange(start, end int64) bool {
	pStart, d, err := k.parse()
	if err != nil {
		return false
	}
	pEnd := pStart.Add(d)
	return start < pEnd.UnixMilli() && end > pStart.UnixMilli()
}

type fullPartition struct {
	PartitionMeta

	shardsMu sync.Mutex
	shards   map[uint32]*indexShard
}

type indexShard struct {
	tenantsMu sync.Mutex
	tenants   map[string]*indexTenant
}

type indexTenant struct {
	blocksMu sync.Mutex
	blocks   map[string]*metastorev1.BlockMeta
}

// LoadPartitions reads all partitions from the backing store and loads the most recent ones in memory.
func (i *Index) LoadPartitions() {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	i.loadedPartitions = make(map[PartitionKey]*fullPartition)
	i.allPartitions = make([]*PartitionMeta, 0)
	for _, key := range i.store.ListPartitions() {
		pMeta, err := i.store.ReadPartitionMeta(key)
		if err != nil {
			level.Error(i.logger).Log("msg", "error reading partition metadata", "key", key, "err", err)
			continue
		}
		i.allPartitions = append(i.allPartitions, pMeta)
		if pMeta.Ts.Add(24 * time.Hour).Before(time.Now()) {
			// too old, will load on demand
			continue
		}
		_, _ = i.getOrLoadPartition(pMeta)
	}

	slices.SortFunc(i.allPartitions, func(a, b *PartitionMeta) int {
		return a.Key.compare(b.Key)
	})
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

func (i *Index) getOrLoadPartition(meta *PartitionMeta) (*fullPartition, error) {
	p, ok := i.loadedPartitions[meta.Key]
	if !ok {
		p := &fullPartition{
			PartitionMeta: *meta,
			shards:        make(map[uint32]*indexShard),
		}
		i.loadedPartitions[meta.Key] = p
		for _, s := range i.store.ListShards(meta.Key) {
			p.shardsMu.Lock()
			sh := &indexShard{
				tenants: make(map[string]*indexTenant),
			}
			p.shards[s] = sh

			for _, t := range i.store.ListTenants(meta.Key, s) {
				sh.tenantsMu.Lock()
				te := &indexTenant{
					blocks: make(map[string]*metastorev1.BlockMeta),
				}
				te.blocksMu.Lock()
				for _, b := range i.store.ListBlocks(meta.Key, s, t) {
					te.blocks[b.Id] = b
				}
				te.blocksMu.Unlock()
				sh.tenants[t] = te
				sh.tenantsMu.Unlock()
			}
			p.shardsMu.Unlock()
		}
	}

	return p, nil
}

func (i *Index) GetPartitionKey(blockId string) PartitionKey {
	t := ulid.Time(ulid.MustParse(blockId).Time()).UTC()
	key := t.Format(dayLayout)
	if i.partitionDuration < 24*time.Hour {
		hour := (t.Hour() / int(i.partitionDuration.Hours())) * int(i.partitionDuration.Hours())
		key = key + fmt.Sprintf("T%02d", hour)
	}
	mDuration := model.Duration(i.partitionDuration)
	key += fmt.Sprintf(".%v", mDuration)
	return PartitionKey(key)
}

func (i *Index) FindPartitionMeta(key PartitionKey) *PartitionMeta {
	loaded, ok := i.loadedPartitions[key]
	if ok {
		return &loaded.PartitionMeta
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
	key := i.GetPartitionKey(b.Id)
	meta, err := i.GetOrCreatePartitionMeta(key)
	if err != nil {
		return err
	}

	if b.TenantId != "" {
		meta.AddTenant(b.TenantId)
	} else {
		for _, ds := range b.Datasets {
			meta.AddTenant(ds.TenantId)
		}
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

func (i *Index) GetOrCreatePartitionMeta(key PartitionKey) (*PartitionMeta, error) {
	meta := i.FindPartitionMeta(key)

	if meta == nil {
		ts, duration, err := key.parse()
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
	meta := i.FindPartitionMeta(key)
	if meta == nil {
		return nil
	}

	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	p, err := i.getOrLoadPartition(meta)
	if err != nil {
		return nil
	}
	s := i.getShard(p, shardNum)
	if s == nil {
		return nil
	}
	t := i.getTenant(s, tenant)
	if t == nil {
		return nil
	}
	b := i.getBlock(t, id)

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
			lastPartitionIdx = idx
		}
	}

	if firstPartitionIdx > 1 {
		meta := i.allPartitions[firstPartitionIdx-1]
		p, err := i.getOrLoadPartition(meta)
		if err != nil {
			level.Error(i.logger).Log("msg", "error loading previous partition", "key", meta.Key, "err", err)
			return nil, err
		}
		tenantBlocks := i.collectTenantBlocks(p, tenants)
		blocks = append(blocks, tenantBlocks...)
	}

	if lastPartitionIdx > 1 && lastPartitionIdx < len(i.allPartitions)-1 {
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

func (i *Index) DeletePartitions(predicate func(*PartitionMeta) bool) []*PartitionMeta {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	var deleted []*PartitionMeta
	n := 0
	for _, p := range i.allPartitions {
		if predicate(p) {
			deleted = append(deleted, p)
			delete(i.loadedPartitions, p.Key)
		} else {
			i.allPartitions[n] = p
			n++
		}
	}
	i.allPartitions = i.allPartitions[:n]
	return deleted
}

func (i *Index) getOrCreatePartition(meta *PartitionMeta) *fullPartition {
	p, ok := i.loadedPartitions[meta.Key]
	if !ok {
		p = &fullPartition{
			PartitionMeta: *meta,
			shards:        make(map[uint32]*indexShard),
		}
		i.loadedPartitions[meta.Key] = p
		i.allPartitions = append(i.allPartitions, meta)
		slices.SortFunc(i.allPartitions, func(a, b *PartitionMeta) int {
			return a.Key.compare(b.Key)
		})
	}
	return p
}

func (i *Index) collectTenantBlocks(p *fullPartition, tenants map[string]struct{}) []*metastorev1.BlockMeta {
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

func (i *Index) getShard(p *fullPartition, shardNum uint32) *indexShard {
	p.shardsMu.Lock()
	defer p.shardsMu.Unlock()
	return p.shards[shardNum]
}

func (i *Index) getTenant(s *indexShard, tenant string) *indexTenant {
	s.tenantsMu.Lock()
	defer s.tenantsMu.Unlock()
	return s.tenants[tenant]
}

func (i *Index) getBlock(t *indexTenant, id string) *metastorev1.BlockMeta {
	t.blocksMu.Lock()
	defer t.blocksMu.Unlock()
	return t.blocks[id]
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
