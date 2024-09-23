package metastore

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

type metastoreIndexStore struct {
	db *boltdb
}

const (
	partitionBucketName   = "partition"
	partitionMetaKeyName  = "meta"
	emptyTenantBucketName = "-"
)

var partitionBucketNameBytes = []byte(partitionBucketName)
var partitionMetaKeyNameBytes = []byte(partitionMetaKeyName)
var emptyTenantBucketNameBytes = []byte(emptyTenantBucketName)

func getPartitionBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	bkt := tx.Bucket(partitionBucketNameBytes)
	if bkt == nil {
		return nil, bbolt.ErrBucketNotFound
	}
	return bkt, nil
}

func (m *metastoreIndexStore) ListPartitions() []index.PartitionKey {
	partitionKeys := make([]index.PartitionKey, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return errors.Wrap(err, "root partition bucket missing")
		}
		err = bkt.ForEachBucket(func(name []byte) error {
			partitionKeys = append(partitionKeys, index.PartitionKey(name))
			return nil
		})
		return err
	})
	if err != nil {
		panic(err)
	}
	return partitionKeys
}

func (m *metastoreIndexStore) ReadPartitionMeta(key index.PartitionKey) (*index.PartitionMeta, error) {
	var meta index.PartitionMeta
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return errors.Wrap(err, "root partition bucket missing")
		}
		partBkt := bkt.Bucket([]byte(key))
		if partBkt == nil {
			return fmt.Errorf("partition meta not found for %s", key)
		}
		data := partBkt.Get(partitionMetaKeyNameBytes)
		dec := gob.NewDecoder(bytes.NewReader(data))
		err = dec.Decode(&meta)
		if err != nil {
			return errors.Wrapf(err, "failed to read partition meta for %s", key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (m *metastoreIndexStore) ListShards(key index.PartitionKey) []uint32 {
	shards := make([]uint32, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return errors.Wrap(err, "root partition bucket missing")
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

func (m *metastoreIndexStore) ListTenants(key index.PartitionKey, shard uint32) []string {
	tenants := make([]string, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return errors.Wrap(err, "root partition bucket missing")
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

func (m *metastoreIndexStore) ListBlocks(key index.PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta {
	blocks := make([]*metastorev1.BlockMeta, 0)
	err := m.db.boltdb.View(func(tx *bbolt.Tx) error {
		bkt, err := getPartitionBucket(tx)
		if err != nil {
			return errors.Wrap(err, "root partition bucket missing")
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

func updateBlockMetadataBucket(tx *bbolt.Tx, partitionMeta *index.PartitionMeta, shard uint32, tenant string, fn func(*bbolt.Bucket) error) error {
	bkt, err := getPartitionBucket(tx)
	if err != nil {
		return errors.Wrap(err, "root partition bucket missing")
	}

	partBkt, err := getOrCreateSubBucket(bkt, []byte(partitionMeta.Key))
	if err != nil {
		return errors.Wrapf(err, "error creating partition bucket for %s", partitionMeta.Key)
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err = enc.Encode(partitionMeta)
	if err != nil {
		return errors.Wrapf(err, "could not encode partition meta for %s", partitionMeta.Key)
	}

	err = partBkt.Put(partitionMetaKeyNameBytes, buf.Bytes())
	if err != nil {
		return errors.Wrapf(err, "could not write partition meta for %s", partitionMeta.Key)
	}

	shardBktName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBktName, shard)
	shardBkt, err := getOrCreateSubBucket(partBkt, shardBktName)
	if err != nil {
		return errors.Wrapf(err, "error creating shard bucket for partiton %s and shard %d", partitionMeta.Key, shard)
	}

	tenantBktName := []byte(tenant)
	if len(tenantBktName) == 0 {
		tenantBktName = emptyTenantBucketNameBytes
	}
	tenantBkt, err := getOrCreateSubBucket(shardBkt, tenantBktName)
	if err != nil {
		return errors.Wrapf(err, "error creating tenant bucket for partition %s, shard %d and tenant %s", partitionMeta.Key, shard, tenant)
	}

	return fn(tenantBkt)
}
