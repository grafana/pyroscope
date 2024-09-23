package index

import (
	"sync"
	"time"

	"github.com/go-kit/log"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type Index struct {
	partitionMu      sync.Mutex
	loadedPartitions map[PartitionKey]*indexPartition
	allPartitions    []*PartitionMeta

	store  Store
	logger log.Logger

	partitionDuration time.Duration
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

type Store interface {
	ListPartitions() []PartitionKey
	ReadPartitionMeta(p PartitionKey) (*PartitionMeta, error)

	ListShards(p PartitionKey) []uint32
	ListTenants(p PartitionKey, shard uint32) []string
	ListBlocks(p PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta

	LoadBlock(p PartitionKey, shard uint32, tenant string, blockId string) *metastorev1.BlockMeta
}
