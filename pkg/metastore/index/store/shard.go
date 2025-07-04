package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/metastore/store"
	multitenancy "github.com/grafana/pyroscope/pkg/tenant"
)

const (
	tenantShardStringsBucketName = ".strings"
	tenantShardIndexKeyName      = ".index"
)

var (
	ErrInvalidStringTable = errors.New("malformed string table")
	ErrInvalidShardIndex  = errors.New("malformed shard index")
)

var (
	tenantShardIndexKeyNameBytes      = []byte(tenantShardIndexKeyName)
	tenantShardStringsBucketNameBytes = []byte(tenantShardStringsBucketName)

	blockCursorSkipPrefix = []byte{'.'}
)

type Shard struct {
	Partition   Partition
	Tenant      string
	Shard       uint32
	ShardIndex  ShardIndex
	StringTable *metadata.StringTable

	// TODO(kolesnikovae): Build a skip index for labels.
	// Labels *metadata.StringTable
}

func NewShard(p Partition, tenant string, shard uint32) *Shard {
	return &Shard{
		Partition:   p,
		Tenant:      tenant,
		Shard:       shard,
		StringTable: metadata.NewStringTable(),
		ShardIndex:  ShardIndex{},
	}
}

func (s *Shard) Store(tx *bbolt.Tx, md *metastorev1.BlockMeta) error {
	shardBucket, err := getOrCreateTenantShardBucket(tx, s.Partition, s.Tenant, s.Shard)
	if err != nil {
		return err
	}

	n := len(s.StringTable.Strings)
	s.StringTable.Import(md)
	if added := s.StringTable.Strings[n:]; len(added) > 0 {
		stringTable, err := getOrCreateSubBucket(shardBucket, tenantShardStringsBucketNameBytes)
		if err != nil {
			return err
		}
		k := binary.BigEndian.AppendUint32(nil, uint32(n))
		v := encodeStrings(added)
		if err = stringTable.Put(k, v); err != nil {
			return err
		}
	}
	md.StringTable = nil
	value, err := md.MarshalVT()
	if err != nil {
		return err
	}

	var updateIndex bool
	if s.ShardIndex.MinTime == 0 || s.ShardIndex.MinTime > md.MinTime {
		s.ShardIndex.MinTime = md.MinTime
		updateIndex = true
	}
	if s.ShardIndex.MaxTime < md.MaxTime {
		s.ShardIndex.MaxTime = md.MaxTime
		updateIndex = true
	}
	if updateIndex {
		if err = shardBucket.Put(tenantShardIndexKeyNameBytes, s.ShardIndex.MarshalBinary()); err != nil {
			return err
		}
	}

	return shardBucket.Put([]byte(md.Id), value)
}

func (s *Shard) Find(tx *bbolt.Tx, blocks ...string) []store.KV {
	bucket := getTenantShardBucket(tx, s.Partition, s.Tenant, s.Shard)
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
	bucket := getTenantShardBucket(tx, s.Partition, s.Tenant, s.Shard)
	if bucket == nil {
		return nil
	}
	cursor := store.NewCursorIter(bucket.Cursor())
	cursor.SkipPrefix = blockCursorSkipPrefix
	return cursor
}

func (s *Shard) Delete(tx *bbolt.Tx, blocks ...string) error {
	tenantShard := getTenantShardBucket(tx, s.Partition, s.Tenant, s.Shard)
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

func (s *Shard) TombstoneName() string {
	var b strings.Builder
	b.WriteString(s.Partition.String())
	b.WriteByte('-')
	b.WriteByte('T')
	if s.Tenant != "" {
		b.WriteString(s.Tenant)
	} else {
		b.WriteString(multitenancy.DefaultTenantID)
	}
	b.WriteByte('-')
	b.WriteByte('S')
	b.WriteString(strconv.FormatUint(uint64(s.Shard), 10))
	return b.String()
}

// ShallowCopy creates a shallow copy: no deep copy of the string table.
// The copy can be accessed safely by multiple readers, and it represents
// a snapshot of the shard including the string table.
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

func getTenantShardBucket(tx *bbolt.Tx, p Partition, tenant string, shard uint32) *bbolt.Bucket {
	if partition := getPartitionsBucket(tx).Bucket(p.Bytes()); partition != nil {
		if shards := partition.Bucket(tenantBucketName(tenant)); shards != nil {
			return shards.Bucket(binary.BigEndian.AppendUint32(nil, shard))
		}
	}
	return nil
}

func getOrCreateTenantShardBucket(tx *bbolt.Tx, p Partition, tenant string, shard uint32) (*bbolt.Bucket, error) {
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

func loadTenantShard(tx *bbolt.Tx, p Partition, tenant string, shard uint32) (*Shard, error) {
	shardBucket := getTenantShardBucket(tx, p, tenant, shard)
	if shardBucket == nil {
		return nil, nil
	}

	s := NewShard(p, tenant, shard)
	stringTable := shardBucket.Bucket(tenantShardStringsBucketNameBytes)
	if stringTable == nil {
		return s, nil
	}
	stringsIter := newStringIter(store.NewCursorIter(stringTable.Cursor()))
	defer func() {
		_ = stringsIter.Close()
	}()
	var err error
	if err = s.StringTable.Load(stringsIter); err != nil {
		return nil, err
	}

	if b := shardBucket.Get(tenantShardIndexKeyNameBytes); len(b) > 0 {
		if err = s.ShardIndex.UnmarshalBinary(b); err != nil {
			return nil, err
		}
	}

	return s, nil
}
