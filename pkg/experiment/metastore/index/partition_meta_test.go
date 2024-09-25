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
		// TODO aleks-p: add more test cases
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
