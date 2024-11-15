package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/test"
)

func TestIndex_GetPartitionKey(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		blockId  string
		want     PartitionKey
	}{
		{
			name:     "1d",
			duration: test.Duration("1d"),
			blockId:  test.ULID("2024-07-15T16:13:43.245Z"),
			want:     PartitionKey("20240715.1d"),
		},
		{
			name:     "1h at start of the window",
			duration: test.Duration("1h"),
			blockId:  test.ULID("2024-07-15T16:00:00.000Z"),
			want:     PartitionKey("20240715T16.1h"),
		},
		{
			name:     "1h in the middle of the window",
			duration: test.Duration("1h"),
			blockId:  test.ULID("2024-07-15T16:13:43.245Z"),
			want:     PartitionKey("20240715T16.1h"),
		},
		{
			name:     "1h at the end of the window",
			duration: test.Duration("1h"),
			blockId:  test.ULID("2024-07-15T16:59:59.999Z"),
			want:     PartitionKey("20240715T16.1h"),
		},
		{
			name:     "6h duration at midnight",
			duration: test.Duration("6h"),
			blockId:  test.ULID("2024-07-15T00:00:00.000Z"),
			want:     PartitionKey("20240715T00.6h"),
		},
		{
			name:     "6h at the middle of a window",
			duration: test.Duration("6h"),
			blockId:  test.ULID("2024-07-15T15:13:43.245Z"),
			want:     PartitionKey("20240715T12.6h"),
		},
		{
			name:     "6h at the end of the window",
			duration: test.Duration("6h"),
			blockId:  test.ULID("2024-07-15T23:59:59.999Z"),
			want:     PartitionKey("20240715T18.6h"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, CreatePartitionKey(tt.blockId, tt.duration), "CreatePartitionKey(%v)", tt.blockId)
		})
	}
}
