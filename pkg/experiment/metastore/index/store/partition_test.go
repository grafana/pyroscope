package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/test"
)

func TestPartition_Overlaps(t *testing.T) {
	type args struct {
		start time.Time
		end   time.Time
	}
	tests := []struct {
		name string
		key  PartitionKey
		args args
		want bool
	}{
		{
			name: "simple overlap",
			key:  NewPartitionKey(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-11T07:15:24.123Z"),
				end:   test.Time("2024-09-11T13:15:24.123Z"),
			},
			want: true,
		},
		{
			name: "overlap at partition start",
			key:  NewPartitionKey(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-11T04:00:00.000Z"),
				end:   test.Time("2024-09-11T06:00:00.000Z"),
			},
			want: true,
		},
		{
			name: "no overlap close to partition start",
			key:  NewPartitionKey(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-11T04:00:00.000Z"),
				end:   test.Time("2024-09-11T05:59:59.999Z"),
			},
			want: false,
		},
		{
			name: "overlap at partition end",
			key:  NewPartitionKey(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-11T11:59:59.999Z"),
				end:   test.Time("2024-09-11T13:00:00.000Z"),
			},
			want: true,
		},
		{
			name: "no overlap close to partition end",
			key:  NewPartitionKey(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-11T12:00:00.000Z"),
				end:   test.Time("2024-09-11T13:59:59.999Z"),
			},
			want: false,
		},
		{
			name: "overlap around midnight",
			key:  NewPartitionKey(test.Time("2024-09-11T00:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-10T19:00:00.000Z"),
				end:   test.Time("2024-09-11T00:01:01.999Z"),
			},
			want: true,
		},
		{
			name: "partition fully contains interval",
			key:  NewPartitionKey(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-11T07:00:00.000Z"),
				end:   test.Time("2024-09-11T08:01:01.999Z"),
			},
			want: true,
		},
		{
			name: "interval fully contains partition",
			key:  NewPartitionKey(test.Time("2024-09-11T06:00:00.000Z"), 6*time.Hour),
			args: args{
				start: test.Time("2024-09-11T02:00:00.000Z"),
				end:   test.Time("2024-09-11T13:01:01.999Z"),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPartition(tt.key)
			assert.Equalf(t, tt.want, p.Overlaps(tt.args.start, tt.args.end), "overlaps(%v, %v)", tt.args.start, tt.args.end)
		})
	}
}

func TestPartitionKey_Equal(t *testing.T) {
	k1 := NewPartitionKey(test.Time("2024-09-11T02:00:00.000Z"), 2*time.Hour)
	k2 := NewPartitionKey(test.Time("2024-09-11T03:01:01.999Z"), 2*time.Hour)
	assert.Equal(t, k1, k2)

	k1 = NewPartitionKey(test.Time("2024-09-11T02:00:00.000Z"), time.Hour)
	k2 = NewPartitionKey(test.Time("2024-09-11T03:01:01.999Z"), time.Hour)
	assert.NotEqual(t, k1, k2)
}

func TestPartitionKey_Encoding(t *testing.T) {
	k := NewPartitionKey(time.Now(), time.Hour*6)
	b, err := k.MarshalBinary()
	assert.NoError(t, err)
	var d PartitionKey
	err = d.UnmarshalBinary(b)
	assert.NoError(t, err)
	assert.Equal(t, k, d)
}
