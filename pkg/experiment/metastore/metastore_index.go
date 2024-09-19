package metastore

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type PartitionKey string

type index struct {
	partitionMu  sync.Mutex
	partitionMap map[PartitionKey]*indexPartition
	partitions   []PartitionKey

	store Store

	partitionDuration time.Duration
}

func NewIndex(store Store) *index {
	return &index{
		partitionMap:      make(map[PartitionKey]*indexPartition),
		partitions:        make([]PartitionKey, 0),
		store:             store,
		partitionDuration: time.Hour,
	}
}

type Store interface {
	ListPartitions() []PartitionKey
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

type indexPartition struct {
	duration time.Duration
	ts       time.Time

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

func (i *index) loadPartitions() {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	i.partitionMap = make(map[PartitionKey]*indexPartition)
	i.partitions = make([]PartitionKey, 0)
	for _, key := range i.store.ListPartitions() {
		i.partitions = append(i.partitions, key)
		ts, _, err := key.parse()
		if err != nil {
			// log
			continue
		}
		if ts.Add(24 * time.Hour).Before(time.Now()) {
			// too old, will load on demand
			continue
		}
		_, _ = i.getOrLoadPartition(key)
	}

	slices.SortFunc(i.partitions, func(a, b PartitionKey) int {
		return a.compare(b)
	})
}

func (i *index) getOrLoadPartition(key PartitionKey) (*indexPartition, error) {
	p, ok := i.partitionMap[key]
	if !ok {
		ts, d, err := key.parse()
		if err != nil {
			return nil, err
		}
		p := &indexPartition{
			shards:   make(map[uint32]*indexShard),
			duration: d,
			ts:       ts,
		}
		i.partitionMap[key] = p
		for _, s := range i.store.ListShards(key) {
			p.shardsMu.Lock()
			sh := &indexShard{
				tenants: make(map[string]*indexTenant),
			}
			p.shards[s] = sh

			for _, t := range i.store.ListTenants(key, s) {
				sh.tenantsMu.Lock()
				te := &indexTenant{
					blocks: make(map[string]*metastorev1.BlockMeta),
				}
				te.blocksMu.Lock()
				for _, b := range i.store.ListBlocks(key, s, t) {
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

func (i *index) getPartitionKey(blockId string) PartitionKey {
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

func (i *index) insertBlock(b *metastorev1.BlockMeta) error {
	key := i.getPartitionKey(b.Id)
	pTime, _, err := key.parse()
	if err != nil {
		return err
	}

	p, ok := i.partitionMap[key]
	if !ok {
		p = &indexPartition{
			duration: i.partitionDuration,
			ts:       pTime,
			shards:   make(map[uint32]*indexShard),
		}
		i.partitionMap[key] = p
		i.partitions = append(i.partitions, key)
		slices.SortFunc(i.partitions, func(a, b PartitionKey) int {
			return a.compare(b)
		})
	}

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

func (i *index) deleteBlock(shard uint32, tenant string, blockId string) {
	key := i.getPartitionKey(blockId)

	p, ok := i.partitionMap[key]
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

func (i *index) run(fn func()) {
	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	fn()
}

// findBlock loads the entire partition if not already loaded
func (i *index) findBlock(shardNum uint32, tenant string, id string) *metastorev1.BlockMeta {
	key := i.getPartitionKey(id)

	i.partitionMu.Lock()
	defer i.partitionMu.Unlock()

	p, err := i.getOrLoadPartition(key)
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

func (i *index) findBlocksInRange(start, end int64, tenants map[string]struct{}) ([]*metastorev1.BlockMeta, error) {
	blocks := make([]*metastorev1.BlockMeta, 0)

	firstPartitionIdx, lastPartitionIdx := -1, -1
	for idx, key := range i.partitions {
		if key.inRange(start, end) {
			if firstPartitionIdx == -1 {
				firstPartitionIdx = idx
			}
			p, err := i.getOrCreatePartition(key)
			if err != nil {
				return nil, err
			}
			tenantBlocks := i.collectTenantBlocks(p, tenants)
			blocks = append(blocks, tenantBlocks...)
		} else if firstPartitionIdx != -1 {
			lastPartitionIdx = idx
		}
	}

	if firstPartitionIdx > 1 {
		p, err := i.getOrCreatePartition(i.partitions[firstPartitionIdx-1])
		if err != nil {
			return nil, err
		}
		tenantBlocks := i.collectTenantBlocks(p, tenants)
		blocks = append(blocks, tenantBlocks...)
	}

	if lastPartitionIdx > 1 && lastPartitionIdx < len(i.partitions)-1 {
		p, err := i.getOrCreatePartition(i.partitions[lastPartitionIdx+1])
		if err != nil {
			return nil, err
		}
		tenantBlocks := i.collectTenantBlocks(p, tenants)
		blocks = append(blocks, tenantBlocks...)
	}

	return blocks, nil
}

func (i *index) getOrCreatePartition(key PartitionKey) (*indexPartition, error) {
	pTime, pDuration, err := key.parse()
	if err != nil {
		return nil, err
	}
	p, ok := i.partitionMap[key]
	if !ok {
		p = &indexPartition{
			ts:       pTime,
			duration: pDuration,
			shards:   make(map[uint32]*indexShard),
		}
		i.partitionMap[key] = p
	}
	return p, nil
}

func (i *index) collectTenantBlocks(p *indexPartition, tenants map[string]struct{}) []*metastorev1.BlockMeta {
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

func (i *index) getShard(p *indexPartition, shardNum uint32) *indexShard {
	p.shardsMu.Lock()
	defer p.shardsMu.Unlock()
	return p.shards[shardNum]
}

func (i *index) getTenant(s *indexShard, tenant string) *indexTenant {
	s.tenantsMu.Lock()
	defer s.tenantsMu.Unlock()
	return s.tenants[tenant]
}

func (i *index) getBlock(t *indexTenant, id string) *metastorev1.BlockMeta {
	t.blocksMu.Lock()
	defer t.blocksMu.Unlock()
	return t.blocks[id]
}

func (i *index) replaceBlocks(sources []string, sourceShard uint32, sourceTenant string, replacements []*metastorev1.BlockMeta) error {
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
