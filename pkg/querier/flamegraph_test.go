package querier

import (
	"fmt"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
)

func Test_ExportToFlamebearer(t *testing.T) {
	expected := &flamebearer.FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
			Metadata: flamebearer.FlamebearerMetadataV1{
				Format:     "single",
				Units:      "bytes",
				Name:       "inuse_space",
				SampleRate: 100,
			},
			Flamebearer: flamebearer.FlamebearerV1{
				Names: []string{"total", "a", "c", "d", "b", "e"},
				Levels: [][]int{
					{0, 4, 0, 0},
					{0, 4, 0, 1},
					{0, 1, 0, 4, 0, 3, 2, 2},
					{0, 1, 1, 5, 2, 1, 1, 3},
				},
				NumTicks: 4,
				MaxSelf:  2,
			},
		},
	}
	actual := ExportToFlamebearer(
		NewFlameGraph(
			newTree([]stacktraces{
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
			}),
			-1,
		), &typesv1.ProfileType{
			ID:         "memory:inuse_space:bytes:space:bytes",
			Name:       "memory",
			SampleType: "inuse_space",
			SampleUnit: "bytes",
			PeriodType: "space",
			PeriodUnit: "bytes",
		})
	require.Equal(t, expected, actual)
}

var f *querierv1.FlameGraph

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
		f = NewFlameGraph(tr, -1)
	}
}
