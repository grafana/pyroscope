package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"iter"
	"time"

	"go.etcd.io/bbolt"
)

var ErrInvalidPartitionKey = errors.New("invalid partition key")

type Partition struct {
	Timestamp time.Time
	Duration  time.Duration
}

func NewPartition(timestamp time.Time, duration time.Duration) Partition {
	return Partition{Timestamp: timestamp.Truncate(duration), Duration: duration}
}

func (p *Partition) Equal(x Partition) bool {
	return p.Timestamp.Equal(x.Timestamp) && p.Duration == x.Duration
}

func (p *Partition) StartTime() time.Time { return p.Timestamp }

func (p *Partition) EndTime() time.Time { return p.Timestamp.Add(p.Duration) }

func (p *Partition) Overlaps(start, end time.Time) bool {
	if start.After(p.EndTime()) {
		return false
	}
	if end.Before(p.StartTime()) {
		return false
	}
	return true
}

func (p *Partition) Bytes() []byte {
	b, _ := p.MarshalBinary()
	return b
}

func (p *Partition) String() string {
	b := make([]byte, 0, 32)
	b = p.Timestamp.UTC().AppendFormat(b, time.DateTime)
	b = append(b, ' ')
	b = append(b, '(')
	b = append(b, p.Duration.String()...)
	b = append(b, ')')
	return string(b)
}

func (p *Partition) MarshalBinary() ([]byte, error) {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[0:8], uint64(p.Timestamp.UnixNano()))
	binary.BigEndian.PutUint32(b[8:12], uint32(p.Duration/time.Second))
	return b, nil
}

func (p *Partition) UnmarshalBinary(b []byte) error {
	if len(b) != 12 {
		return ErrInvalidPartitionKey
	}
	p.Timestamp = time.Unix(0, int64(binary.BigEndian.Uint64(b[0:8])))
	p.Duration = time.Duration(binary.BigEndian.Uint32(b[8:12])) * time.Second
	return nil
}

func (p *Partition) Query(tx *bbolt.Tx) *PartitionQuery {
	b := getPartitionsBucket(tx).Bucket(p.Bytes())
	if b == nil {
		return nil
	}
	return &PartitionQuery{
		tx:        tx,
		Partition: *p,
		bucket:    b,
	}
}

type PartitionQuery struct {
	Partition
	tx     *bbolt.Tx
	bucket *bbolt.Bucket
}

func (q *PartitionQuery) Tenants() iter.Seq[string] {
	return func(yield func(string) bool) {
		cursor := q.bucket.Cursor()
		for tenantKey, _ := cursor.First(); tenantKey != nil; tenantKey, _ = cursor.Next() {
			tenantBucket := q.bucket.Bucket(tenantKey)
			if tenantBucket == nil {
				continue
			}
			tenant := string(tenantKey)
			if bytes.Equal(tenantKey, emptyTenantBucketNameBytes) {
				tenant = ""
			}
			if !yield(tenant) {
				return
			}
		}
	}
}

func (q *PartitionQuery) Shards(tenant string) iter.Seq[Shard] {
	tenantBucket := q.bucket.Bucket(tenantBucketName(tenant))
	if tenantBucket == nil {
		return func(func(Shard) bool) {}
	}
	return func(yield func(Shard) bool) {
		cursor := tenantBucket.Cursor()
		for shardKey, _ := cursor.First(); shardKey != nil; shardKey, _ = cursor.Next() {
			shardBucket := tenantBucket.Bucket(shardKey)
			if shardBucket == nil {
				continue
			}
			shard := Shard{
				Partition:  q.Partition,
				Tenant:     tenant,
				Shard:      binary.BigEndian.Uint32(shardKey),
				ShardIndex: ShardIndex{},
			}
			if b := shardBucket.Get(tenantShardIndexKeyNameBytes); len(b) > 0 {
				if err := shard.ShardIndex.UnmarshalBinary(b); err != nil {
					continue
				}
			}
			if !yield(shard) {
				return
			}
		}
	}
}
