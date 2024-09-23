package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPartitionKey_inRange(t *testing.T) {
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

func createTime(t string) time.Time {
	ts, _ := time.Parse(time.RFC3339, t)
	return ts
}
