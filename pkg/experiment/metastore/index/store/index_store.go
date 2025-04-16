package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

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
	tenantShardIndexKeyName      = ".index"
)

var (
	ErrInvalidStringTable = errors.New("malformed string table")
	ErrInvalidShardIndex  = errors.New("malformed shard index")
)

var (
	partitionBucketNameBytes          = []byte(partitionBucketName)
	emptyTenantBucketNameBytes        = []byte(emptyTenantBucketName)
	tenantShardStringsBucketNameBytes = []byte(tenantShardStringsBucketName)
	tenantShardIndexKeyNameBytes      = []byte(tenantShardIndexKeyName)

	blockCursorSkipPrefix = []byte{'.'}
)

type Entry struct {
	Partition   PartitionKey
	Shard       uint32
	Tenant      string
	BlockID     string
	BlockMeta   *metastorev1.BlockMeta
	StringTable *metadata.StringTable
}

type Shard struct {
	Partition PartitionKey
	Tenant    string
	Shard     uint32
	*metadata.StringTable
	ShardIndex
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
				// TODO: Store tenant shard index in the Partition
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

func getTenantShard(tx *bbolt.Tx, p PartitionKey, tenant string, shard uint32) *bbolt.Bucket {
	if partition := getPartitionsBucket(tx).Bucket(p.Bytes()); partition != nil {
		if shards := partition.Bucket(tenantBucketName(tenant)); shards != nil {
			return shards.Bucket(binary.BigEndian.AppendUint32(nil, shard))
		}
	}
	return nil
}

func (m *IndexStore) LoadShard(tx *bbolt.Tx, p PartitionKey, tenant string, shard uint32) (*Shard, error) {
	s, err := m.loadTenantShard(tx, p, tenant, shard)
	if err != nil {
		return nil, fmt.Errorf("error loading tenant shard %s/%d partition %q: %w", tenant, shard, p, err)
	}
	return s, nil
}

func (m *IndexStore) loadTenantShard(tx *bbolt.Tx, p PartitionKey, tenant string, shard uint32) (*Shard, error) {
	tenantShard := getTenantShard(tx, p, tenant, shard)
	if tenantShard == nil {
		return nil, nil
	}

	s := Shard{
		Partition:   p,
		Tenant:      tenant,
		Shard:       shard,
		ShardIndex:  ShardIndex{},
		StringTable: metadata.NewStringTable(),
	}

	strings := tenantShard.Bucket(tenantShardStringsBucketNameBytes)
	if strings == nil {
		return &s, nil
	}
	stringsIter := newStringIter(store.NewCursorIter(strings.Cursor()))
	defer func() {
		_ = stringsIter.Close()
	}()
	var err error
	if err = s.StringTable.Load(stringsIter); err != nil {
		return nil, err
	}

	if b := tenantShard.Get(tenantShardIndexKeyNameBytes); len(b) > 0 {
		if err = s.ShardIndex.UnmarshalBinary(b); err != nil {
			return nil, err
		}
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

func (s *Shard) Store(tx *bbolt.Tx, md *metastorev1.BlockMeta) error {
	bucket, err := getOrCreateTenantShard(tx, s.Partition, s.Tenant, s.Shard)
	if err != nil {
		return err
	}
	n := len(s.StringTable.Strings)
	s.StringTable.Import(md)
	if added := s.StringTable.Strings[n:]; len(added) > 0 {
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

	var updateIndex bool
	if s.MinTime == 0 || s.MinTime > md.MinTime {
		s.MinTime = md.MinTime
		updateIndex = true
	}
	if s.MaxTime < md.MaxTime {
		s.MaxTime = md.MaxTime
		updateIndex = true
	}
	if updateIndex {
		if err = bucket.Put(tenantShardIndexKeyNameBytes, s.ShardIndex.MarshalBinary()); err != nil {
			return err
		}
	}

	return bucket.Put([]byte(md.Id), value)
}

func (s *Shard) Find(tx *bbolt.Tx, blocks ...string) []store.KV {
	bucket := getTenantShard(tx, s.Partition, s.Tenant, s.Shard)
	if bucket == nil {
		return nil
	}
	kv := make([]store.KV, 0, len(blocks))
	for _, b := range blocks {
		k := []byte(b)
		if v := bucket.Get(k); v != nil {
			kv = append(kv, store.KV{Key: k, Value: v})
		}
	}
	return kv
}

func (s *Shard) Blocks(tx *bbolt.Tx) *store.CursorIterator {
	bucket := getTenantShard(tx, s.Partition, s.Tenant, s.Shard)
	if bucket == nil {
		return nil
	}
	cursor := store.NewCursorIter(bucket.Cursor())
	cursor.SkipPrefix = blockCursorSkipPrefix
	return cursor
}

func (s *Shard) Delete(tx *bbolt.Tx, blocks ...string) error {
	tenantShard := getTenantShard(tx, s.Partition, s.Tenant, s.Shard)
	if tenantShard == nil {
		return nil
	}
	for _, b := range blocks {
		if err := tenantShard.Delete([]byte(b)); err != nil {
			return err
		}
	}
	return nil
}

// ShallowCopy creates a shallow copy: no deep copy of the string table.
// The copy can be accessed safely by multiple readers, and it represents
// a snapshot of the string table at the time of the copy.
//
// Strings added after the copy is made won't be visible to the reader.
// The writer MUST invalidate the cache before access: copies in-use can
// still be used (strings is a header copy of append-only slice).
func (s *Shard) ShallowCopy() *Shard {
	return &Shard{
		Partition:  s.Partition,
		Tenant:     s.Tenant,
		Shard:      s.Shard,
		ShardIndex: s.ShardIndex,
		StringTable: &metadata.StringTable{
			Strings: s.StringTable.Strings,
		},
	}
}

func (s *Shard) Overlaps(start, end time.Time) bool {
	// For backward compatibility.
	if s.MinTime == 0 || s.MaxTime == 0 {
		return true
	}
	return start.Before(time.UnixMilli(s.MaxTime)) && !end.Before(time.UnixMilli(s.MinTime))
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

type ShardIndex struct {
	MinTime int64
	MaxTime int64
}

func (i *ShardIndex) UnmarshalBinary(data []byte) error {
	if len(data) != 16 {
		return ErrInvalidShardIndex
	}
	i.MinTime = int64(binary.BigEndian.Uint64(data[0:8]))
	i.MaxTime = int64(binary.BigEndian.Uint64(data[8:16]))
	return nil
}

func (i *ShardIndex) MarshalBinary() []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b[0:8], uint64(i.MinTime))
	binary.BigEndian.PutUint64(b[8:16], uint64(i.MaxTime))
	return b
}
