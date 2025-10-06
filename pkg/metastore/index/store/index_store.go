package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	goiter "iter"

	"go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"
)

const (
	partitionBucketName   = "partition"
	emptyTenantBucketName = "-"
)

var (
	partitionBucketNameBytes   = []byte(partitionBucketName)
	emptyTenantBucketNameBytes = []byte(emptyTenantBucketName)
)

type IndexStore struct{}

func tenantBucketName(tenant string) []byte {
	if tenant == "" {
		return emptyTenantBucketNameBytes
	}
	return []byte(tenant)
}

func getPartitionsBucket(tx *bbolt.Tx) *bbolt.Bucket {
	return tx.Bucket(partitionBucketNameBytes)
}

func getOrCreateSubBucket(parent *bbolt.Bucket, name []byte) (*bbolt.Bucket, error) {
	bucket := parent.Bucket(name)
	if bucket == nil {
		return parent.CreateBucket(name)
	}
	return bucket, nil
}

func NewIndexStore() *IndexStore {
	return &IndexStore{}
}

func (m *IndexStore) CreateBuckets(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(partitionBucketNameBytes)
	return err
}

func (m *IndexStore) Partitions(tx *bbolt.Tx) goiter.Seq[Partition] {
	root := getPartitionsBucket(tx)
	if root == nil {
		return func(func(Partition) bool) {}
	}
	return func(yield func(Partition) bool) {
		cursor := root.Cursor()
		for partitionKey, _ := cursor.First(); partitionKey != nil; partitionKey, _ = cursor.Next() {
			p := Partition{}
			if err := p.UnmarshalBinary(partitionKey); err != nil {
				continue
			}
			if !yield(p) {
				return
			}
		}
	}
}

func (m *IndexStore) LoadShard(tx *bbolt.Tx, p Partition, tenant string, shard uint32) (*Shard, error) {
	s, err := loadTenantShard(tx, p, tenant, shard)
	if err != nil {
		return nil, fmt.Errorf("error loading tenant shard %s/%d partition %q: %w", tenant, shard, p, err)
	}
	return s, nil
}

func (m *IndexStore) DeleteShard(tx *bbolt.Tx, p Partition, tenant string, shard uint32) error {
	partitions := getPartitionsBucket(tx)
	partitionKey := p.Bytes()
	if partition := partitions.Bucket(partitionKey); partition != nil {
		tenantKey := tenantBucketName(tenant)
		if shards := partition.Bucket(tenantKey); shards != nil {
			if err := shards.DeleteBucket(binary.BigEndian.AppendUint32(nil, shard)); err != nil {
				if !errors.Is(err, bbolterrors.ErrBucketNotFound) {
					return err
				}
			}
			if isBucketEmpty(shards) {
				if err := partition.DeleteBucket(tenantKey); err != nil {
					if !errors.Is(err, bbolterrors.ErrBucketNotFound) {
						return err
					}
				}
			}
		}
		if isBucketEmpty(partition) {
			if err := partitions.DeleteBucket(partitionKey); err != nil {
				if !errors.Is(err, bbolterrors.ErrBucketNotFound) {
					return err
				}
			}
		}
	}
	return nil
}

func isBucketEmpty(bucket *bbolt.Bucket) bool {
	if bucket == nil {
		return true
	}
	c := bucket.Cursor()
	k, _ := c.First()
	return k == nil
}
