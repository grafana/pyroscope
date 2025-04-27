package store

import (
	"cmp"
	"encoding/binary"
	"errors"
	"slices"
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

func (p *Partition) StartTime() time.Time               { return p.Key.StartTime() }
func (p *Partition) EndTime() time.Time                 { return p.Key.EndTime() }
func (p *Partition) Overlaps(start, end time.Time) bool { return p.Key.Overlaps(start, end) }

func (k *PartitionKey) StartTime() time.Time { return k.Timestamp }
func (k *PartitionKey) EndTime() time.Time   { return k.Timestamp.Add(k.Duration) }
func (k *PartitionKey) Overlaps(start, end time.Time) bool {
	return start.Before(k.EndTime()) && !end.Before(k.StartTime())
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

func (p *Partition) HasIndexShard(tenant string, shard uint32) bool {
	t, ok := p.TenantShards[tenant]
	if ok {
		_, ok = t[shard]
	}
	return ok
}

func (p *Partition) Compare(other *Partition) int {
	if p == other {
		return 0
	}
	return p.Key.Timestamp.Compare(other.Key.Timestamp)
}

func (p *Partition) Shards(dst []Shard) []Shard {
	dst = dst[:0]
	for tenant, shards := range p.TenantShards {
		for shard := range shards {
			dst = append(dst, Shard{
				Partition: p.Key,
				Tenant:    tenant,
				Shard:     shard,
			})
		}
	}
	slices.SortFunc(dst, func(a, b Shard) int {
		t := cmp.Compare(a.Tenant, b.Tenant)
		if t != 0 {
			return cmp.Compare(a.Shard, b.Shard)
		}
		return t
	})
	return dst
}

func (p *Partition) DeleteTenantShard(tenant string, shard uint32) {
	if t := p.TenantShards[tenant]; t != nil {
		delete(t, shard)
		if len(t) == 0 {
			delete(p.TenantShards, tenant)
		}
	}
}

func (p *Partition) IsEmpty() bool {
	return len(p.TenantShards) == 0
}

func NewPartitionKey(timestamp time.Time, duration time.Duration) PartitionKey {
	return PartitionKey{Timestamp: timestamp.Truncate(duration), Duration: duration}
}

func (k *PartitionKey) Equal(x PartitionKey) bool {
	return k.Timestamp.Equal(x.Timestamp) && k.Duration == x.Duration
}

func (k *PartitionKey) MarshalBinary() ([]byte, error) {
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

func (k *PartitionKey) Bytes() []byte {
	b, _ := k.MarshalBinary()
	return b
}

func (k *PartitionKey) String() string {
	b := make([]byte, 0, 32)
	b = k.Timestamp.UTC().AppendFormat(b, time.DateTime)
	b = append(b, ' ')
	b = append(b, '(')
	b = append(b, k.Duration.String()...)
	b = append(b, ')')
	return string(b)
}
