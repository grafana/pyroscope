package metastore

import (
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

type indexStore struct {
	db     *boltdb
	logger log.Logger
}

func newIndexStore(db *boltdb, logger log.Logger) index.Store {
	return &indexStore{
		db:     db,
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
	if bkt == nil {
		return nil, bbolt.ErrBucketNotFound
	}
	return bkt, nil
}

func (m *indexStore) ListPartitions() []index.PartitionKey {
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
		level.Error(m.logger).Log("msg", "error listing partitions", "err", err)
		panic(err)
	}
	return partitionKeys
}

func (m *indexStore) ListShards(key index.PartitionKey) []uint32 {
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
		level.Error(m.logger).Log("msg", "error listing shards", "partition", key, "err", err)
		panic(err)
	}
	return shards
}

func (m *indexStore) ListTenants(key index.PartitionKey, shard uint32) []string {
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
		level.Error(m.logger).Log("msg", "error listing tenants", "partition", key, "shard", shard, "err", err)
		panic(err)
	}
	return tenants
}

func (m *indexStore) ListBlocks(key index.PartitionKey, shard uint32, tenant string) []*metastorev1.BlockMeta {
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
		level.Error(m.logger).Log("msg", "error listing blocks", "partition", key, "shard", shard, "tenant", tenant, "err", err)
		panic(err)
	}
	return blocks
}

func updateBlockMetadataBucket(tx *bbolt.Tx, partKey index.PartitionKey, shard uint32, tenant string, fn func(*bbolt.Bucket) error) error {
	bkt, err := getPartitionBucket(tx)
	if err != nil {
		return errors.Wrap(err, "root partition bucket missing")
	}

	partBkt, err := getOrCreateSubBucket(bkt, []byte(partKey))
	if err != nil {
		return errors.Wrapf(err, "error creating partition bucket for %s", partKey)
	}

	shardBktName := make([]byte, 4)
	binary.BigEndian.PutUint32(shardBktName, shard)
	shardBkt, err := getOrCreateSubBucket(partBkt, shardBktName)
	if err != nil {
		return errors.Wrapf(err, "error creating shard bucket for partiton %s and shard %d", partKey, shard)
	}

	tenantBktName := []byte(tenant)
	if len(tenantBktName) == 0 {
		tenantBktName = emptyTenantBucketNameBytes
	}
	tenantBkt, err := getOrCreateSubBucket(shardBkt, tenantBktName)
	if err != nil {
		return errors.Wrapf(err, "error creating tenant bucket for partition %s, shard %d and tenant %s", partKey, shard, tenant)
	}

	return fn(tenantBkt)
}
