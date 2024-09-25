package index

import (
	"flag"
	"sync"
	"time"

	"github.com/go-kit/log"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

var DefaultConfig = Config{
	PartitionDuration: time.Hour,
	PartitionTTL:      4 * time.Hour,
	CleanupInterval:   5 * time.Minute,
}

type Config struct {
	PartitionDuration time.Duration `yaml:"partition_duration"`
	PartitionTTL      time.Duration `yaml:"partition_ttl"`
	CleanupInterval   time.Duration `yaml:"cleanup_interval"`
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.PartitionDuration, prefix+"partition-duration", DefaultConfig.PartitionDuration, "")
	f.DurationVar(&cfg.PartitionTTL, prefix+"partition-ttl", DefaultConfig.PartitionTTL, "")
	f.DurationVar(&cfg.CleanupInterval, prefix+"cleanup-interval", DefaultConfig.CleanupInterval, "")
}

type Index struct {
	Config *Config

	partitionMu      sync.Mutex
	loadedPartitions map[PartitionKey]*indexPartition
	allPartitions    []*PartitionMeta

	store  Store
	logger log.Logger
}

type indexPartition struct {
	meta     *PartitionMeta
	loadedAt time.Time

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
