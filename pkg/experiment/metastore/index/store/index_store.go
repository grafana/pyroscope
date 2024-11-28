package store

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

const (
	partitionBucketName   = "partition"
	emptyTenantBucketName = "-"
)

var (
	partitionBucketNameBytes   = []byte(partitionBucketName)
	emptyTenantBucketNameBytes = []byte(emptyTenantBucketName)
)

type PartitionBlocks struct {
	Tenant  string
	Shard   uint32
	Blocks  []*metastorev1.BlockMeta
	Strings []string
}

type IndexStore struct{}

func NewIndexStore() *IndexStore {
	return &IndexStore{}
}

func getPartitionBucket(tx *bbolt.Tx) *bbolt.Bucket {
	return tx.Bucket(partitionBucketNameBytes)
}

func (m *IndexStore) CreateBuckets(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(partitionBucketNameBytes)
	return err
}

func (m *IndexStore) StoreBlock(tx *bbolt.Tx, pk PartitionKey, b *metastorev1.BlockMeta) error {
	key := []byte(b.StringTable[b.Id])
	value, err := b.MarshalVT()
	if err != nil {
		return err
	}
	partBkt, err := getOrCreateSubBucket(getPartitionBucket(tx), []byte(pk))
	if err != nil {
		return fmt.Errorf("error creating partition bucket for %s: %w", pk, err)
	}

	shardBktName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBktName, b.Shard)
	shardBkt, err := getOrCreateSubBucket(partBkt, shardBktName)
	if err != nil {
		return fmt.Errorf("error creating shard bucket for partiton %s and shard %d: %w", pk, b.Shard, err)
	}

	tenantBktName := []byte(b.StringTable[b.Tenant])
	if len(tenantBktName) == 0 {
		tenantBktName = emptyTenantBucketNameBytes
	}
	tenantBkt, err := getOrCreateSubBucket(shardBkt, tenantBktName)
	if err != nil {
		return err
	}

	return tenantBkt.Put(key, value)
}

func (m *IndexStore) DeleteBlockList(tx *bbolt.Tx, pk PartitionKey, list *metastorev1.BlockList) error {
	partitions := getPartitionBucket(tx)
	if partitions == nil {
		return nil
	}
	partition := partitions.Bucket([]byte(pk))
	if partition == nil {
		return nil
	}
	shardBktName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBktName, list.Shard)
	shards := partition.Bucket(shardBktName)
	if shards == nil {
		return nil
	}
	tenantBktName := []byte(list.Tenant)
	if len(tenantBktName) == 0 {
		tenantBktName = emptyTenantBucketNameBytes
	}
	tenant := shards.Bucket(tenantBktName)
	if tenant == nil {
		return nil
	}
	for _, b := range list.Blocks {
		if err := tenant.Delete([]byte(b)); err != nil {
			return err
		}
	}
	return nil
}

func (m *IndexStore) ListPartitions(tx *bbolt.Tx) []PartitionKey {
	partitionKeys := make([]PartitionKey, 0)
	_ = getPartitionBucket(tx).ForEachBucket(func(name []byte) error {
		partitionKeys = append(partitionKeys, PartitionKey(name))
		return nil
	})
	return partitionKeys
}

func (m *IndexStore) ListShards(tx *bbolt.Tx, key PartitionKey) []uint32 {
	shards := make([]uint32, 0)
	partBkt := getPartitionBucket(tx).Bucket([]byte(key))
	if partBkt == nil {
		return nil
	}
	_ = partBkt.ForEachBucket(func(name []byte) error {
		shards = append(shards, binary.BigEndian.Uint32(name))
		return nil
	})
	return shards
}

func (m *IndexStore) ListTenants(tx *bbolt.Tx, key PartitionKey, shard uint32) []string {
	tenants := make([]string, 0)
	partBkt := getPartitionBucket(tx).Bucket([]byte(key))
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
		if bytes.Equal(name, emptyTenantBucketNameBytes) {
			tenants = append(tenants, "")
		} else {
			tenants = append(tenants, string(name))
		}
		return nil
	})
	return tenants
}

func (m *IndexStore) ListBlocks(tx *bbolt.Tx, key PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta {
	blocks := make([]*metastorev1.BlockMeta, 0)
	partBkt := getPartitionBucket(tx).Bucket([]byte(key))
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

func getOrCreateSubBucket(parent *bbolt.Bucket, name []byte) (*bbolt.Bucket, error) {
	bucket := parent.Bucket(name)
	if bucket == nil {
		return parent.CreateBucket(name)
	}
	return bucket, nil
}
