package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPartitionMeta_overlaps(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		meta PartitionMeta
		args args
		want bool
	}{
		{
			name: "simple overlap",
			meta: PartitionMeta{Ts: createTime("2024-09-11T06:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-11T07:15:24.123Z"),
				end:   createTime("2024-09-11T13:15:24.123Z"),
			},
			want: true,
		},
		{
			name: "overlap at partition start",
			meta: PartitionMeta{Ts: createTime("2024-09-11T06:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-11T04:00:00.000Z"),
				end:   createTime("2024-09-11T06:00:00.000Z"),
			},
			want: true,
		},
		{
			name: "no overlap close to partition start",
			meta: PartitionMeta{Ts: createTime("2024-09-11T06:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-11T04:00:00.000Z"),
				end:   createTime("2024-09-11T05:59:59.999Z"),
			},
			want: false,
		},
		{
			name: "overlap at partition end",
			meta: PartitionMeta{Ts: createTime("2024-09-11T06:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-11T11:59:59.999Z"),
				end:   createTime("2024-09-11T13:00:00.000Z"),
			},
			want: true,
		},
		{
			name: "no overlap close to partition end",
			meta: PartitionMeta{Ts: createTime("2024-09-11T06:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-11T12:00:00.000Z"),
				end:   createTime("2024-09-11T13:59:59.999Z"),
			},
			want: false,
		},
		{
			name: "overlap around midnight",
			meta: PartitionMeta{Ts: createTime("2024-09-11T00:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-10T19:00:00.000Z"),
				end:   createTime("2024-09-11T00:01:01.999Z"),
			},
			want: true,
		},
		{
			name: "partition fully contains interval",
			meta: PartitionMeta{Ts: createTime("2024-09-11T06:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-11T07:00:00.000Z"),
				end:   createTime("2024-09-11T08:01:01.999Z"),
			},
			want: true,
		},
		{
			name: "interval fully contains partition",
			meta: PartitionMeta{Ts: createTime("2024-09-11T06:00:00.000Z"), Duration: 6 * time.Hour},
			args: args{
				start: createTime("2024-09-11T02:00:00.000Z"),
				end:   createTime("2024-09-11T13:01:01.999Z"),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.meta.overlaps(tt.args.start.UnixMilli(), tt.args.end.UnixMilli()), "overlaps(%v, %v)", tt.args.start, tt.args.end)
		})
	}
}

func createTime(t string) time.Time {
	ts, _ := time.Parse(time.RFC3339, t)
	return ts
}
