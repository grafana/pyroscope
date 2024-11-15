package index

import (
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/go-kit/log"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/storeutils"
)

type indexStore struct {
	logger log.Logger
}

func NewIndexStore(logger log.Logger) Store {
	return &indexStore{
		logger: logger,
	}
}

const (
	partitionBucketName   = "partition"
	emptyTenantBucketName = "-"
)

var partitionBucketNameBytes = []byte(partitionBucketName)
var emptyTenantBucketNameBytes = []byte(emptyTenantBucketName)

func getPartitionBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	bkt := tx.Bucket(partitionBucketNameBytes)
	return bkt, nil
}

func (m *indexStore) ListPartitions(tx *bbolt.Tx) []PartitionKey {
	partitionKeys := make([]PartitionKey, 0)
	bkt, err := getPartitionBucket(tx)
	if err != nil {
		return nil
	}
	_ = bkt.ForEachBucket(func(name []byte) error {
		partitionKeys = append(partitionKeys, PartitionKey(name))
		return nil
	})
	return partitionKeys
}

func (m *indexStore) ListShards(tx *bbolt.Tx, key PartitionKey) []uint32 {
	shards := make([]uint32, 0)
	bkt, err := getPartitionBucket(tx)
	if err != nil {
		return nil
	}
	partBkt := bkt.Bucket([]byte(key))
	if partBkt == nil {
		return nil
	}
	_ = partBkt.ForEachBucket(func(name []byte) error {
		shards = append(shards, binary.BigEndian.Uint32(name))
		return nil
	})
	return shards
}

func (m *indexStore) ListTenants(tx *bbolt.Tx, key PartitionKey, shard uint32) []string {
	tenants := make([]string, 0)
	bkt, err := getPartitionBucket(tx)
	if err != nil {
		return nil
	}
	partBkt := bkt.Bucket([]byte(key))
	if partBkt == nil {
		return nil
	}
	shardBktName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBktName, shard)
	shardBkt := partBkt.Bucket(shardBktName)
	if shardBkt == nil {
		return nil
	}
	_ = shardBkt.ForEachBucket(func(name []byte) error {
		if slices.Equal(name, emptyTenantBucketNameBytes) {
			tenants = append(tenants, "")
		} else {
			tenants = append(tenants, string(name))
		}
		return nil
	})
	return tenants
}

func (m *indexStore) ListBlocks(tx *bbolt.Tx, key PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta {
	blocks := make([]*metastorev1.BlockMeta, 0)
	bkt, err := getPartitionBucket(tx)
	if err != nil {
		return nil
	}
	partBkt := bkt.Bucket([]byte(key))
	if partBkt == nil {
		return nil
	}
	shardBktName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBktName, shard)
	shardBkt := partBkt.Bucket(shardBktName)
	if shardBkt == nil {
		return nil
	}
	tenantBktName := []byte(tenant)
	if len(tenantBktName) == 0 {
		tenantBktName = emptyTenantBucketNameBytes
	}
	tenantBkt := shardBkt.Bucket(tenantBktName)
	if tenantBkt == nil {
		return nil
	}
	_ = tenantBkt.ForEach(func(k, v []byte) error {
		var md metastorev1.BlockMeta
		if err := md.UnmarshalVT(v); err != nil {
			panic(fmt.Sprintf("failed to unmarshal block %q: %v", string(k), err))
		}
		blocks = append(blocks, &md)
		return nil
	})
	return blocks
}

func UpdateBlockMetadataBucket(tx *bbolt.Tx, partKey PartitionKey, shard uint32, tenant string, fn func(*bbolt.Bucket) error) error {
	bkt, err := getPartitionBucket(tx)
	if err != nil {
		return errors.Wrap(err, "root partition bucket missing")
	}

	partBkt, err := storeutils.GetOrCreateSubBucket(bkt, []byte(partKey))
	if err != nil {
		return errors.Wrapf(err, "error creating partition bucket for %s", partKey)
	}

	shardBktName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBktName, shard)
	shardBkt, err := storeutils.GetOrCreateSubBucket(partBkt, shardBktName)
	if err != nil {
		return errors.Wrapf(err, "error creating shard bucket for partiton %s and shard %d", partKey, shard)
	}

	tenantBktName := []byte(tenant)
	if len(tenantBktName) == 0 {
		tenantBktName = emptyTenantBucketNameBytes
	}
	tenantBkt, err := storeutils.GetOrCreateSubBucket(shardBkt, tenantBktName)
	if err != nil {
		return errors.Wrapf(err, "error creating tenant bucket for partition %s, shard %d and tenant %s", partKey, shard, tenant)
	}

	return fn(tenantBkt)
}
