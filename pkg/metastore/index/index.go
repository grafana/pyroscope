package index

import (
	"context"
	"flag"
	"fmt"
	"iter"
	"math"
	"slices"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid/v2"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/metastore/index/cleaner"
	"github.com/grafana/pyroscope/pkg/metastore/index/dlq"
	indexstore "github.com/grafana/pyroscope/pkg/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/model"
)

var ErrBlockExists = fmt.Errorf("block already exists")

type Config struct {
	ShardCacheSize      int `yaml:"shard_cache_size"`
	BlockWriteCacheSize int `yaml:"block_write_cache_size"`
	BlockReadCacheSize  int `yaml:"block_read_cache_size"`

	Cleaner  cleaner.Config `yaml:",inline"`
	Recovery dlq.Config     `yaml:",inline"`

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
	cfg.Recovery.RegisterFlagsWithPrefix(prefix, f)
	cfg.Cleaner.RegisterFlagsWithPrefix(prefix, f)
	f.IntVar(&cfg.ShardCacheSize, prefix+"shard-cache-size", DefaultConfig.ShardCacheSize, "Maximum number of shards to keep in memory")
	f.IntVar(&cfg.BlockWriteCacheSize, prefix+"block-write-cache-size", DefaultConfig.BlockWriteCacheSize, "Maximum number of written blocks to keep in memory")
	f.IntVar(&cfg.BlockReadCacheSize, prefix+"block-read-cache-size", DefaultConfig.BlockReadCacheSize, "Maximum number of read blocks to keep in memory")
	cfg.partitionDuration = DefaultConfig.partitionDuration
	cfg.queryLookaroundPeriod = DefaultConfig.queryLookaroundPeriod
}

type Store interface {
	CreateBuckets(*bbolt.Tx) error
	Partitions(tx *bbolt.Tx) iter.Seq[indexstore.Partition]
	LoadShard(tx *bbolt.Tx, p indexstore.Partition, tenant string, shard uint32) (*indexstore.Shard, error)
	DeleteShard(tx *bbolt.Tx, p indexstore.Partition, tenant string, shard uint32) error
}

type Index struct {
	logger log.Logger
	config Config
	store  Store
	shards *shardCache
	blocks *blockCache
}

func NewIndex(logger log.Logger, s Store, cfg Config) *Index {
	return &Index{
		logger: logger,
		config: cfg,
		store:  s,
		shards: newShardCache(cfg.ShardCacheSize, s),
		blocks: newBlockCache(cfg.BlockReadCacheSize, cfg.BlockWriteCacheSize),
	}
}

func NewStore() *indexstore.IndexStore { return indexstore.NewIndexStore() }

func (i *Index) Init(tx *bbolt.Tx) error { return i.store.CreateBuckets(tx) }

func (i *Index) Restore(tx *bbolt.Tx) error {
	// See comment in DefaultConfig.queryLookaroundPeriod.
	now := time.Now()
	start := now.Add(-i.config.queryLookaroundPeriod)
	end := now.Add(i.config.queryLookaroundPeriod)
	for p := range i.store.Partitions(tx) {
		if !p.Overlaps(start, end) {
			continue
		}
		level.Info(i.logger).Log("msg", "loading partition in memory")
		q := p.Query(tx)
		if q == nil {
			continue
		}
		for tenant := range q.Tenants() {
			for shard := range q.Shards(tenant) {
				if _, err := i.shards.getForWrite(tx, p, tenant, shard.Shard); err != nil {
					level.Error(i.logger).Log(
						"msg", "failed to load tenant partition shard",
						"partition", p,
						"tenant", tenant,
						"shard", shard,
						"err", err,
					)
					return err
				}
			}
		}
	}
	return nil
}

func (i *Index) InsertBlock(tx *bbolt.Tx, b *metastorev1.BlockMeta) error {
	p := i.partitionKeyForBlock(b.Id)
	return i.shards.update(tx, p, metadata.Tenant(b), b.Shard, func(s *indexstore.Shard) error {
		if err := s.Store(tx, b); err != nil {
			return err
		}
		i.blocks.put(s, b)
		return nil
	})
}

func (i *Index) ReplaceBlocks(tx *bbolt.Tx, compacted *metastorev1.CompactedBlocks) error {
	for _, b := range compacted.NewBlocks {
		if err := i.InsertBlock(tx, b); err != nil {
			return err
		}
	}
	for p, list := range i.partitionedList(compacted.SourceBlocks) {
		err := i.shards.update(tx, p, list.Tenant, list.Shard, func(s *indexstore.Shard) error {
			if err := s.Delete(tx, list.Blocks...); err != nil {
				return err
			}
			for _, b := range list.Blocks {
				i.blocks.delete(s, b)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *Index) GetBlocks(tx *bbolt.Tx, list *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error) {
	metas := make([]*metastorev1.BlockMeta, 0, len(list.Blocks))
	for k, partitioned := range i.partitionedList(list) {
		s, err := i.shards.getForRead(tx, k, partitioned.Tenant, partitioned.Shard)
		if err != nil {
			return nil, err
		}
		for _, kv := range s.Find(tx, partitioned.Blocks...) {
			b := i.blocks.getOrCreate(s, kv).CloneVT()
			s.StringTable.Export(b)
			metas = append(metas, b)
		}
	}
	return metas, nil
}

func (i *Index) Partitions(tx *bbolt.Tx) iter.Seq[indexstore.Partition] {
	return i.store.Partitions(tx)
}

func (i *Index) DeleteShard(tx *bbolt.Tx, key indexstore.Partition, tenant string, shard uint32) error {
	if err := i.store.DeleteShard(tx, key, tenant, shard); err != nil {
		return err
	}
	i.shards.delete(key, tenant, shard)
	return nil
}

func (i *Index) GetTenants(tx *bbolt.Tx) []string {
	uniqueTenants := make(map[string]struct{})
	for p := range i.store.Partitions(tx) {
		q := p.Query(tx)
		if q == nil {
			// Partition not found.
			continue
		}
		for t := range q.Tenants() {
			if t == "" {
				continue
			}
			uniqueTenants[t] = struct{}{}
		}
	}
	tenants := make([]string, 0, len(uniqueTenants))
	for t := range uniqueTenants {
		tenants = append(tenants, t)
	}
	return tenants
}

func (i *Index) GetTenantStats(tx *bbolt.Tx, tenant string) *metastorev1.TenantStats {
	stats := &metastorev1.TenantStats{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}
	for p := range i.store.Partitions(tx) {
		q := p.Query(tx)
		if q == nil {
			// Partition not found.
			continue
		}
		for shard := range q.Shards(tenant) {
			stats.DataIngested = true
			oldest := shard.ShardIndex.MinTime
			newest := shard.ShardIndex.MaxTime
			if oldest < stats.OldestProfileTime {
				stats.OldestProfileTime = oldest
			}
			if newest > stats.NewestProfileTime {
				stats.NewestProfileTime = newest
			}
		}
	}
	if !stats.DataIngested {
		return new(metastorev1.TenantStats)
	}
	return stats
}

func (i *Index) QueryMetadata(tx *bbolt.Tx, ctx context.Context, query MetadataQuery) ([]*metastorev1.BlockMeta, error) {
	q, err := newMetadataQuery(i, query)
	if err != nil {
		return nil, err
	}
	r, err := newBlockMetadataQuerier(tx, q).queryBlocks(ctx)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (i *Index) QueryMetadataLabels(tx *bbolt.Tx, ctx context.Context, query MetadataQuery) ([]*typesv1.Labels, error) {
	q, err := newMetadataQuery(i, query)
	if err != nil {
		return nil, err
	}
	c, err := newMetadataLabelQuerier(tx, q).queryLabels(ctx)
	if err != nil {
		return nil, err
	}
	l := slices.Collect(c.Unique())
	slices.SortFunc(l, model.CompareLabels)
	return l, nil
}

func (i *Index) partitionedList(list *metastorev1.BlockList) map[indexstore.Partition]*metastorev1.BlockList {
	partitions := make(map[indexstore.Partition]*metastorev1.BlockList)
	for _, b := range list.Blocks {
		k := i.partitionKeyForBlock(b)
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

func (i *Index) partitionKeyForBlock(b string) indexstore.Partition {
	return indexstore.NewPartition(ulid.Time(ulid.MustParse(b).Time()), i.config.partitionDuration)
}
