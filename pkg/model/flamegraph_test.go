package model

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

func Test_FlameGraphMerger(t *testing.T) {
	t.Run("two non-empty flamegraphs", func(t *testing.T) {
		m := NewFlameGraphMerger()
		s := new(Tree)
		s.InsertStack(1, "foo", "bar")
		s.InsertStack(1, "foo", "bar", "baz")
		s.InsertStack(2, "foo", "qux")
		s.InsertStack(1, "fred", "zoo")
		s.InsertStack(1, "fred", "other")
		m.MergeFlameGraph(NewFlameGraph(s, -1))

		s = new(Tree)
		s.InsertStack(1, "foo", "bar", "baz")
		s.InsertStack(1, "fred", "zoo")
		s.InsertStack(1, "fred", "zoo", "ward")
		s.InsertStack(1, "func", "other")
		s.InsertStack(1, "func")
		s.InsertStack(1, "other")
		m.MergeFlameGraph(NewFlameGraph(s, -1))

		expected := new(Tree)
		expected.InsertStack(1, "foo", "bar")
		expected.InsertStack(1, "foo", "bar", "baz")
		expected.InsertStack(2, "foo", "qux")
		expected.InsertStack(1, "fred", "zoo")
		expected.InsertStack(1, "fred", "other")
		expected.InsertStack(1, "foo", "bar", "baz")
		expected.InsertStack(1, "fred", "zoo")
		expected.InsertStack(1, "fred", "zoo", "ward")
		expected.InsertStack(1, "func", "other")
		expected.InsertStack(1, "func")
		expected.InsertStack(1, "other")

		require.Equal(t, expected.String(), m.Tree().String())
	})

	t.Run("non-empty flamegraph result truncation", func(t *testing.T) {
		m := NewFlameGraphMerger()
		s := new(Tree)
		s.InsertStack(1, "foo", "bar")
		s.InsertStack(1, "foo", "bar", "baz")
		s.InsertStack(2, "foo", "qux")
		s.InsertStack(1, "fred", "zoo")
		s.InsertStack(1, "fred", "other")
		m.MergeFlameGraph(NewFlameGraph(s, -1))

		fg := m.FlameGraph(4)
		m = NewFlameGraphMerger()
		m.MergeFlameGraph(fg)

		expected := new(Tree)
		expected.InsertStack(1, "foo", "bar")
		expected.InsertStack(1, "foo", "bar", "other")
		expected.InsertStack(2, "foo", "qux")
		expected.InsertStack(2, "fred", "other")

		require.Equal(t, expected.String(), m.Tree().String())
	})

	t.Run("empty flamegraph", func(t *testing.T) {
		m := NewFlameGraphMerger()
		m.MergeFlameGraph(NewFlameGraph(new(Tree), -1))
		require.Equal(t, new(Tree).String(), m.Tree().String())
	})

	t.Run("no flamegraphs", func(t *testing.T) {
		m := NewFlameGraphMerger()
		require.Equal(t, new(Tree).String(), m.Tree().String())
	})
}
