package index

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
)

func TestIndex_getPartitionKey(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		blockId  string
		want     PartitionKey
	}{
		{
			name:     "1d",
			duration: createDuration("1d"),
			blockId:  createUlidString("2024-07-15T16:13:43.245Z"),
			want:     PartitionKey("20240715.1d"),
		},
		{
			name:     "1h at start of the window",
			duration: createDuration("1h"),
			blockId:  createUlidString("2024-07-15T16:00:00.000Z"),
			want:     PartitionKey("20240715T16.1h"),
		},
		{
			name:     "1h in the middle of the window",
			duration: createDuration("1h"),
			blockId:  createUlidString("2024-07-15T16:13:43.245Z"),
			want:     PartitionKey("20240715T16.1h"),
		},
		{
			name:     "1h at the end of the window",
			duration: createDuration("1h"),
			blockId:  createUlidString("2024-07-15T16:59:59.999Z"),
			want:     PartitionKey("20240715T16.1h"),
		},
		{
			name:     "6h duration at midnight",
			duration: createDuration("6h"),
			blockId:  createUlidString("2024-07-15T00:00:00.000Z"),
			want:     PartitionKey("20240715T00.6h"),
		},
		{
			name:     "6h at the middle of a window",
			duration: createDuration("6h"),
			blockId:  createUlidString("2024-07-15T15:13:43.245Z"),
			want:     PartitionKey("20240715T12.6h"),
		},
		{
			name:     "6h at the end of the window",
			duration: createDuration("6h"),
			blockId:  createUlidString("2024-07-15T23:59:59.999Z"),
			want:     PartitionKey("20240715T18.6h"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &Index{
				loadedPartitions:  make(map[PartitionKey]*fullPartition),
				partitionDuration: tt.duration,
			}
			assert.Equalf(t, tt.want, i.GetPartitionKey(tt.blockId), "getPartitionKey(%v)", tt.blockId)
		})
	}
}

func createDuration(d string) time.Duration {
	parsed, _ := model.ParseDuration(d)
	return time.Duration(parsed)
}

func createTime(t string) time.Time {
	ts, _ := time.Parse(time.RFC3339, t)
	return ts
}

func createUlidString(t string) string {
	parsed, _ := time.Parse(time.RFC3339, t)
	l := ulid.MustNew(ulid.Timestamp(parsed), rand.Reader)
	return l.String()
}

func TestPartitionKey_Covers(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		k    PartitionKey
		args args
		want bool
	}{
		{
			name: "simple overlapping",
			k:    "20240911T06.6h",
			args: args{
				start: createTime("2024-09-11T07:15:24.123Z"),
				end:   createTime("2024-09-11T13:15:24.123Z"),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.k.inRange(tt.args.start.UnixMilli(), tt.args.end.UnixMilli()), "inRange(%v, %v)", tt.args.start, tt.args.end)
		})
	}
}
