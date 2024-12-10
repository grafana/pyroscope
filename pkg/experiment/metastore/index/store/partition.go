package store

import (
	"encoding/binary"
	"errors"
	"time"
)

var ErrInvalidPartitionKey = errors.New("invalid partition key")

type Partition struct {
	Key          PartitionKey
	TenantShards map[string]map[uint32]struct{}
}

type PartitionKey struct {
	Timestamp time.Time
	Duration  time.Duration
}

func NewPartition(k PartitionKey) *Partition {
	return &Partition{
		Key:          k,
		TenantShards: make(map[string]map[uint32]struct{}),
	}
}

func (p *Partition) StartTime() time.Time { return p.Key.Timestamp }
func (p *Partition) EndTime() time.Time   { return p.Key.Timestamp.Add(p.Key.Duration) }

func (p *Partition) Overlaps(start, end time.Time) bool {
	return start.Before(p.EndTime()) && !end.Before(p.StartTime())
}

func (p *Partition) AddTenantShard(tenant string, shard uint32) {
	t := p.TenantShards[tenant]
	if t == nil {
		t = make(map[uint32]struct{})
		p.TenantShards[tenant] = t
	}
	t[shard] = struct{}{}
}

func (p *Partition) HasTenant(t string) bool {
	_, ok := p.TenantShards[t]
	return ok
}

func (p *Partition) Compare(other *Partition) int {
	if p == other {
		return 0
	}
	return p.Key.Timestamp.Compare(other.Key.Timestamp)
}

func NewPartitionKey(timestamp time.Time, duration time.Duration) PartitionKey {
	return PartitionKey{Timestamp: timestamp.Truncate(duration), Duration: duration}
}

func (k PartitionKey) Equal(x PartitionKey) bool {
	return k.Timestamp.Equal(x.Timestamp) && k.Duration == x.Duration
}

func (k PartitionKey) MarshalBinary() ([]byte, error) {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[0:8], uint64(k.Timestamp.UnixNano()))
	binary.BigEndian.PutUint32(b[8:12], uint32(k.Duration/time.Second))
	return b, nil
}

func (k *PartitionKey) UnmarshalBinary(b []byte) error {
	if len(b) != 12 {
		return ErrInvalidPartitionKey
	}
	k.Timestamp = time.Unix(0, int64(binary.BigEndian.Uint64(b[0:8])))
	k.Duration = time.Duration(binary.BigEndian.Uint32(b[8:12])) * time.Second
	return nil
}

func (k PartitionKey) Bytes() []byte {
	b, _ := k.MarshalBinary()
	return b
}

func (k PartitionKey) String() string {
	b := make([]byte, 0, 32)
	b = k.Timestamp.UTC().AppendFormat(b, time.DateTime)
	b = append(b, ' ')
	b = append(b, '(')
	b = append(b, k.Duration.String()...)
	b = append(b, ')')
	return string(b)
}
