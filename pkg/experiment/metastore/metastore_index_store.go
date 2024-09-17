package metastore

import (
	"encoding/binary"
	"fmt"
	"slices"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type metastoreIndexStore struct {
	db *boltdb
}

const (
	partitionBucketName = "partition"
)

var partitionBucketNameBytes = []byte(partitionBucketName)

func getPartitionBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	bkt := tx.Bucket(partitionBucketNameBytes)
	if bkt == nil {
		return nil, bbolt.ErrBucketNotFound
	}
	return bkt, nil
}

func (m *metastoreIndexStore) ListPartitions() []PartitionKey {
	partitionKeys := make([]PartitionKey, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return err
		}
		err = bkt.ForEachBucket(func(name []byte) error {
			partitionKeys = append(partitionKeys, PartitionKey(name))
			return nil
		})
		return err
	})
	if err != nil {
		panic(err)
	}
	return partitionKeys
}

func (m *metastoreIndexStore) ListShards(key PartitionKey) []uint32 {
	shards := make([]uint32, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return err
		}
		partBkt := bkt.Bucket([]byte(key))
		if partBkt == nil {
			return nil
		}
		return partBkt.ForEachBucket(func(name []byte) error {
			shards = append(shards, binary.BigEndian.Uint32(name))
			return nil
		})
	})
	if err != nil {
		panic(err)
	}
	return shards
}

func (m *metastoreIndexStore) ListTenants(key PartitionKey, shard uint32) []string {
	tenants := make([]string, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return err
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
		return shardBkt.ForEachBucket(func(name []byte) error {
			if slices.Equal(name, emptyTenantBucketNameBytes) {
				tenants = append(tenants, "")
			} else {
				tenants = append(tenants, string(name))
			}
			return nil
		})
	})
	if err != nil {
		panic(err)
	}
	return tenants
}

func (m *metastoreIndexStore) ListBlocks(key PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta {
	blocks := make([]*metastorev1.BlockMeta, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return err
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
		return tenantBkt.ForEach(func(k, v []byte) error {
			var md metastorev1.BlockMeta
			if err := md.UnmarshalVT(v); err != nil {
				return fmt.Errorf("failed to unmarshal block %q: %w", string(k), err)
			}
			blocks = append(blocks, &md)
			return nil
		})
	})
	if err != nil {
		panic(err)
	}
	return blocks
}

func (m *metastoreIndexStore) LoadBlock(key PartitionKey, shard uint32, tenant string, blockId string) *metastorev1.BlockMeta {
	var block *metastorev1.BlockMeta
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return err
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
		tenantBkt := shardBkt.Bucket([]byte(tenant))
		if tenantBkt == nil {
			return nil
		}
		blockData := tenantBkt.Get([]byte(blockId))
		if blockData == nil {
			return nil
		}
		var md metastorev1.BlockMeta
		if err := md.UnmarshalVT(blockData); err != nil {
			return fmt.Errorf("failed to unmarshal block %q: %w", blockId, err)
		}
		block = &md
		return nil
	})
	if err != nil {
		panic(err)
	}
	return block
}
