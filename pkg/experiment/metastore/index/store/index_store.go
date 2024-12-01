package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

// TODO(kolesnikovae): Handle ALL errors; don't panic in the read path.

const (
	partitionBucketName          = "partition"
	emptyTenantBucketName        = "-"
	tenantShardStringsBucketName = ".strings"
)

var (
	partitionBucketNameBytes          = []byte(partitionBucketName)
	emptyTenantBucketNameBytes        = []byte(emptyTenantBucketName)
	tenantShardStringsBucketNameBytes = []byte(tenantShardStringsBucketName)
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

func (m *IndexStore) CreateBuckets(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(partitionBucketNameBytes)
	return err
}

func (m *IndexStore) StoreBlock(tx *bbolt.Tx, pk PartitionKey, shard uint32, tenant, id string, md *metastorev1.BlockMeta) error {
	tenantShard, err := getOrCreateTenantShard(tx, pk, shard, tenant)
	if err != nil {
		return err
	}
	value, err := md.MarshalVT()
	if err != nil {
		return err
	}
	return tenantShard.Put([]byte(id), value)
}

func (m *IndexStore) ListPartitions(tx *bbolt.Tx) []PartitionKey {
	partitions := make([]PartitionKey, 0)
	_ = getPartition(tx).ForEachBucket(func(name []byte) error {
		partitions = append(partitions, PartitionKey(name))
		return nil
	})
	return partitions
}

func (m *IndexStore) ListShards(tx *bbolt.Tx, key PartitionKey) []uint32 {
	shards := make([]uint32, 0)
	partition := getPartition(tx).Bucket([]byte(key))
	if partition == nil {
		return nil
	}
	_ = partition.ForEachBucket(func(name []byte) error {
		shards = append(shards, binary.BigEndian.Uint32(name))
		return nil
	})
	return shards
}

func (m *IndexStore) ListTenants(tx *bbolt.Tx, key PartitionKey, shard uint32) []string {
	tenants := make([]string, 0)
	partition := getPartition(tx).Bucket([]byte(key))
	if partition == nil {
		return nil
	}
	shardBkt := partition.Bucket(binary.BigEndian.AppendUint32(nil, shard))
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
	tenantShard := getTenantShard(tx, key, shard, tenant)
	if tenantShard == nil {
		return nil
	}
	blocks := make([]*metastorev1.BlockMeta, 0)
	_ = tenantShard.ForEach(func(k, v []byte) error {
		var md metastorev1.BlockMeta
		if err := md.UnmarshalVT(v); err != nil {
			panic(fmt.Sprintf("failed to unmarshal block %q: %v", string(k), err))
		}
		blocks = append(blocks, &md)
		return nil
	})
	return blocks
}

func (m *IndexStore) DeleteBlockList(tx *bbolt.Tx, pk PartitionKey, list *metastorev1.BlockList) error {
	tenantShard := getTenantShard(tx, pk, list.Shard, list.Tenant)
	for _, b := range list.Blocks {
		if err := tenantShard.Delete([]byte(b)); err != nil {
			return err
		}
	}
	return nil
}

func getPartition(tx *bbolt.Tx) *bbolt.Bucket {
	return tx.Bucket(partitionBucketNameBytes)
}

func getOrCreateSubBucket(parent *bbolt.Bucket, name []byte) (*bbolt.Bucket, error) {
	bucket := parent.Bucket(name)
	if bucket == nil {
		return parent.CreateBucket(name)
	}
	return bucket, nil
}

func getTenantShard(tx *bbolt.Tx, key PartitionKey, shard uint32, tenant string) *bbolt.Bucket {
	partition := getPartition(tx).Bucket([]byte(key))
	if partition == nil {
		return nil
	}
	shards := partition.Bucket(binary.BigEndian.AppendUint32(nil, shard))
	if shards == nil {
		return nil
	}
	if tenant == "" {
		tenant = emptyTenantBucketName
	}
	tenantShard := shards.Bucket([]byte(tenant))
	if tenantShard == nil {
		return nil
	}
	return tenantShard
}

func getOrCreateTenantShard(tx *bbolt.Tx, key PartitionKey, shard uint32, tenant string) (*bbolt.Bucket, error) {
	partition, err := getOrCreateSubBucket(getPartition(tx), []byte(key))
	if err != nil {
		return nil, fmt.Errorf("error creating partition bucket for %s: %w", key, err)
	}
	shards, err := getOrCreateSubBucket(partition, binary.BigEndian.AppendUint32(nil, shard))
	if err != nil {
		return nil, fmt.Errorf("error creating shard bucket for partiton %s and shard %d: %w", key, shard, err)
	}
	if tenant == "" {
		tenant = emptyTenantBucketName
	}
	tenantShard, err := getOrCreateSubBucket(shards, []byte(tenant))
	if err != nil {
		return nil, fmt.Errorf("error creating shard bucket for tenant %s in parititon %v: %w", tenant, key, err)
	}
	return tenantShard, nil
}

func (m *IndexStore) StoreStrings(tx *bbolt.Tx, p PartitionKey, shard uint32, tenant string, offset int, s []string) error {
	tenantShard, err := getOrCreateTenantShard(tx, p, shard, tenant)
	if err != nil {
		return err
	}
	strings, err := getOrCreateSubBucket(tenantShard, tenantShardStringsBucketNameBytes)
	if err != nil {
		return err
	}
	return strings.Put(binary.BigEndian.AppendUint32(nil, uint32(offset)), encodeStrings(s))
}

func (m *IndexStore) LoadStrings(tx *bbolt.Tx, p PartitionKey, shard uint32, tenant string) iter.Iterator[string] {
	tenantShard := getTenantShard(tx, p, shard, tenant)
	if tenantShard == nil {
		return iter.NewEmptyIterator[string]()
	}
	strings := tenantShard.Bucket(tenantShardStringsBucketNameBytes)
	if strings == nil {
		return iter.NewEmptyIterator[string]()
	}
	return newStringIter(store.NewCursorIter(nil, strings.Cursor()))
}

var ErrInvalidStringTable = errors.New("malformed string table")

type stringIterator struct {
	iter.Iterator[store.KV]
	batch []string
	cur   int
	err   error
}

func newStringIter(i iter.Iterator[store.KV]) *stringIterator {
	return &stringIterator{Iterator: i}
}

func (i *stringIterator) Err() error {
	if err := i.Iterator.Err(); err != nil {
		return err
	}
	return i.err
}

func (i *stringIterator) At() string { return i.batch[i.cur] }

func (i *stringIterator) Next() bool {
	if n := i.cur + 1; n < len(i.batch) {
		i.cur = n
		return true
	}
	i.cur = 0
	i.batch = i.batch[:0]
	if !i.Iterator.Next() {
		return false
	}
	var err error
	if i.batch, err = decodeStrings(i.batch, i.Iterator.At().Value); err != nil {
		i.err = err
		return false
	}
	return len(i.batch) > 0
}

func encodeStrings(strings []string) []byte {
	size := 4
	for _, s := range strings {
		size += 4 + len(s)
	}
	data := make([]byte, 0, size)
	data = binary.BigEndian.AppendUint32(data, uint32(len(strings)))
	for _, s := range strings {
		data = binary.BigEndian.AppendUint32(data, uint32(len(s)))
		data = append(data, s...)
	}
	return data
}

func decodeStrings(dst []string, data []byte) ([]string, error) {
	offset := 0
	if len(data) < offset+4 {
		return dst, ErrInvalidStringTable
	}
	n := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	for i := uint32(0); i < n; i++ {
		if len(data) < offset+4 {
			return dst, ErrInvalidStringTable
		}
		size := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		if len(data) < offset+int(size) {
			return dst, ErrInvalidStringTable
		}
		strData := data[offset : offset+int(size)]
		if !utf8.Valid(strData) {
			return dst, ErrInvalidStringTable
		}
		dst = append(dst, string(strData))
		offset += int(size)
	}
	return dst, nil
}
