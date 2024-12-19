package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/store"
	"github.com/grafana/pyroscope/pkg/iter"
)

const (
	partitionBucketName          = "partition"
	emptyTenantBucketName        = "-"
	tenantShardStringsBucketName = ".strings"
)

var (
	ErrInvalidStringTable = errors.New("malformed string table")
)

var (
	partitionBucketNameBytes          = []byte(partitionBucketName)
	emptyTenantBucketNameBytes        = []byte(emptyTenantBucketName)
	tenantShardStringsBucketNameBytes = []byte(tenantShardStringsBucketName)
)

type Entry struct {
	Partition   PartitionKey
	Shard       uint32
	Tenant      string
	BlockID     string
	BlockMeta   *metastorev1.BlockMeta
	StringTable *metadata.StringTable
}

type TenantShard struct {
	Partition   PartitionKey
	Tenant      string
	Shard       uint32
	Blocks      []*metastorev1.BlockMeta
	StringTable *metadata.StringTable
}

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

func (m *IndexStore) StoreBlock(tx *bbolt.Tx, shard *TenantShard, md *metastorev1.BlockMeta) error {
	bucket, err := getOrCreateTenantShard(tx, shard.Partition, shard.Tenant, shard.Shard)
	if err != nil {
		return err
	}
	n := len(shard.StringTable.Strings)
	shard.StringTable.Import(md)
	if added := shard.StringTable.Strings[n:]; len(added) > 0 {
		strings, err := getOrCreateSubBucket(bucket, tenantShardStringsBucketNameBytes)
		if err != nil {
			return err
		}
		k := binary.BigEndian.AppendUint32(nil, uint32(n))
		v := encodeStrings(added)
		if err = strings.Put(k, v); err != nil {
			return err
		}
	}
	md.StringTable = nil
	value, err := md.MarshalVT()
	if err != nil {
		return err
	}
	return bucket.Put([]byte(md.Id), value)
}

func (m *IndexStore) ListPartitions(tx *bbolt.Tx) ([]*Partition, error) {
	var partitions []*Partition
	root := getPartitionsBucket(tx)
	return partitions, root.ForEachBucket(func(partitionKey []byte) error {
		var k PartitionKey
		if err := k.UnmarshalBinary(partitionKey); err != nil {
			return fmt.Errorf("%w: %x", err, partitionKey)
		}

		p := NewPartition(k)
		partition := root.Bucket(partitionKey)
		err := partition.ForEachBucket(func(tenant []byte) error {
			shards := make(map[uint32]struct{})
			err := partition.Bucket(tenant).ForEachBucket(func(shard []byte) error {
				shards[binary.BigEndian.Uint32(shard)] = struct{}{}
				return nil
			})
			if err != nil {
				return err
			}
			if bytes.Compare(tenant, emptyTenantBucketNameBytes) == 0 {
				tenant = nil
			}
			p.TenantShards[string(tenant)] = shards
			return nil
		})

		if err != nil {
			return err
		}

		partitions = append(partitions, p)
		return nil
	})
}

func (m *IndexStore) DeleteBlockList(tx *bbolt.Tx, p PartitionKey, list *metastorev1.BlockList) error {
	tenantShard := getTenantShard(tx, p, list.Tenant, list.Shard)
	if tenantShard == nil {
		return nil
	}
	for _, b := range list.Blocks {
		if err := tenantShard.Delete([]byte(b)); err != nil {
			return err
		}
	}
	return nil
}

func getTenantShard(tx *bbolt.Tx, p PartitionKey, tenant string, shard uint32) *bbolt.Bucket {
	if partition := getPartitionsBucket(tx).Bucket(p.Bytes()); partition != nil {
		if shards := partition.Bucket(tenantBucketName(tenant)); shards != nil {
			return shards.Bucket(binary.BigEndian.AppendUint32(nil, shard))
		}
	}
	return nil
}

func (m *IndexStore) LoadTenantShard(tx *bbolt.Tx, p PartitionKey, tenant string, shard uint32) (*TenantShard, error) {
	s, err := m.loadTenantShard(tx, p, tenant, shard)
	if err != nil {
		return nil, fmt.Errorf("error loading tenant shard %s/%d partition %q: %w", tenant, shard, p, err)
	}
	return s, nil
}

func (m *IndexStore) loadTenantShard(tx *bbolt.Tx, p PartitionKey, tenant string, shard uint32) (*TenantShard, error) {
	tenantShard, err := getOrCreateTenantShard(tx, p, tenant, shard)
	if err != nil {
		return nil, err
	}

	s := TenantShard{
		Partition:   p,
		Tenant:      tenant,
		Shard:       shard,
		StringTable: metadata.NewStringTable(),
	}

	strings := tenantShard.Bucket(tenantShardStringsBucketNameBytes)
	if strings == nil {
		return &s, nil
	}
	stringsIter := newStringIter(store.NewCursorIter(nil, strings.Cursor()))
	defer func() {
		_ = stringsIter.Close()
	}()
	if err = s.StringTable.Load(stringsIter); err != nil {
		return nil, err
	}

	err = tenantShard.ForEach(func(k, v []byte) error {
		var md metastorev1.BlockMeta
		if err = md.UnmarshalVT(v); err != nil {
			return fmt.Errorf("failed to unmarshal block %q: %w", string(k), err)
		}
		s.Blocks = append(s.Blocks, &md)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &s, nil
}

func getOrCreateTenantShard(tx *bbolt.Tx, p PartitionKey, tenant string, shard uint32) (*bbolt.Bucket, error) {
	partition, err := getOrCreateSubBucket(getPartitionsBucket(tx), p.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error creating partition bucket for %s: %w", p, err)
	}
	shards, err := getOrCreateSubBucket(partition, tenantBucketName(tenant))
	if err != nil {
		return nil, fmt.Errorf("error creating shard bucket for tenant %s in parititon %v: %w", tenant, p, err)
	}
	tenantShard, err := getOrCreateSubBucket(shards, binary.BigEndian.AppendUint32(nil, shard))
	if err != nil {
		return nil, fmt.Errorf("error creating shard bucket for partiton %s and shard %d: %w", p, shard, err)
	}
	return tenantShard, nil
}

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
		dst = append(dst, string(data[offset:offset+int(size)]))
		offset += int(size)
	}
	return dst, nil
}
