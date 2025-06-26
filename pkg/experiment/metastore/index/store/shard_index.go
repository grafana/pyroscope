package store

import (
	"encoding/binary"
	"time"
)

type ShardIndex struct {
	MinTime int64
	MaxTime int64
}

func (i *ShardIndex) UnmarshalBinary(data []byte) error {
	if len(data) < 16 {
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

func (i *ShardIndex) Overlaps(start, end time.Time) bool {
	// For backward compatibility.
	if i.MinTime == 0 || i.MaxTime == 0 {
		return true
	}
	if start.After(time.UnixMilli(i.MaxTime)) {
		return false
	}
	if end.Before(time.UnixMilli(i.MinTime)) {
		return false
	}
	return true
}
