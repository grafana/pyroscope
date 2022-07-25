package querier

import (
	"fmt"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/stretchr/testify/require"
)

func Test_toFlamebearer(t *testing.T) {
	require.Equal(t, &flamebearer.FlamebearerV1{
		Names: []string{"total", "a", "c", "d", "b", "e"},
		Levels: [][]int{
			{0, 4, 0, 0},
			{0, 4, 0, 1},
			{0, 1, 0, 4, 0, 3, 2, 2},
			{0, 1, 1, 5, 2, 1, 1, 3},
		},
		NumTicks: 4,
		MaxSelf:  2,
	}, NewFlamebearer(newTree([]stacktraces{
		{
			locations: []string{"e", "b", "a"},
			value:     1,
		},
		{
			locations: []string{"c", "a"},
			value:     2,
		},
		{
			locations: []string{"d", "c", "a"},
			value:     1,
		},
	})))
}

var f *flamebearer.FlamebearerV1

func BenchmarkFlamegraph(b *testing.B) {
	stacks := make([]stacktraces, 2000)
	for i := range stacks {
		stacks[i] = stacktraces{
			locations: []string{"a", "b", "c", "d", "e", fmt.Sprintf("%d", i)},
			value:     1,
		}
	}
	tr := newTree(stacks)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f = NewFlamebearer(tr)
	}
}
