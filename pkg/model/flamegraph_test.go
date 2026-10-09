package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/og/structs/flamebearer"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func emptyMapping() map[string]string {
    return map[string]string{}
}

func Test_ExportToFlamebearer(t *testing.T) {
	pType := &typesv1.ProfileType{
		ID:         "memory:inuse_space:bytes:space:bytes",
		Name:       "memory",
		SampleType: "inuse_space",
		SampleUnit: "bytes",
		PeriodType: "space",
		PeriodUnit: "bytes",
	}
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
            emptyMapping(),
			-1,
		), pType)
	require.Equal(t, expected, actual)

	t.Run("nil", func(t *testing.T) {
		require.Equal(t, &flamebearer.FlamebearerProfile{
			Version: 1,
			FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
				Metadata: flamebearer.FlamebearerMetadataV1{
					Format:     "single",
					SampleRate: 100,
					Units:      "bytes",
					Name:       "inuse_space",
				},
				Flamebearer: flamebearer.FlamebearerV1{
					Levels: [][]int{},
				},
			},
		}, ExportToFlamebearer(nil, pType))
	})
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
		f = NewFlameGraph(tr, emptyMapping(), -1)
	}
}

func Test_FlameGraphMerger(t *testing.T) {
	t.Run("two non-empty flamegraphs", func(t *testing.T) {
		m := NewFlameGraphMerger()
		s := new(FunctionNameTree)
		s.InsertStack(1, "foo", "bar")
		s.InsertStack(1, "foo", "bar", "baz")
		s.InsertStack(2, "foo", "qux")
		s.InsertStack(1, "fred", "zoo")
		s.InsertStack(1, "fred", "other")
		m.MergeFlameGraph(NewFlameGraph(s, emptyMapping(), -1))

		s = new(FunctionNameTree)
		s.InsertStack(1, "foo", "bar", "baz")
		s.InsertStack(1, "fred", "zoo")
		s.InsertStack(1, "fred", "zoo", "ward")
		s.InsertStack(1, "func", "other")
		s.InsertStack(1, "func")
		s.InsertStack(1, "other")
		m.MergeFlameGraph(NewFlameGraph(s, emptyMapping(), -1))

		expected := new(FunctionNameTree)
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
		s := new(FunctionNameTree)
		s.InsertStack(1, "foo", "bar")
		s.InsertStack(1, "foo", "bar", "baz")
		s.InsertStack(2, "foo", "qux")
		s.InsertStack(1, "fred", "zoo")
		s.InsertStack(1, "fred", "other")
		m.MergeFlameGraph(NewFlameGraph(s, emptyMapping(), -1))

		fg := m.FlameGraph(4)
		m = NewFlameGraphMerger()
		m.MergeFlameGraph(fg)

		expected := new(FunctionNameTree)
		expected.InsertStack(1, "foo", "bar")
		expected.InsertStack(1, "foo", "bar", "other")
		expected.InsertStack(2, "foo", "qux")
		expected.InsertStack(2, "fred", "other")

		require.Equal(t, expected.String(), m.Tree().String())
	})

	t.Run("empty flamegraph", func(t *testing.T) {
		m := NewFlameGraphMerger()
		m.MergeFlameGraph(NewFlameGraph(new(FunctionNameTree), emptyMapping(), -1))
		require.Equal(t, new(FunctionNameTree).String(), m.Tree().String())
	})

	t.Run("no flamegraphs", func(t *testing.T) {
		m := NewFlameGraphMerger()
		require.Equal(t, new(FunctionNameTree).String(), m.Tree().String())
	})
}

func Test_NewFlameGraph_Mappings(t *testing.T) {
    t.Run("mappings are correctly assigned to names", func(t *testing.T) {
        s := new(FunctionNameTree)
        s.InsertStack(1, "foo", "bar")
        s.InsertStack(1, "foo", "baz")

        nameToMapping := map[string]string{
            "foo": "foo_file",
            "bar": "bar_file",
            "baz": "baz_file",
        }

        fg := NewFlameGraph(s, nameToMapping, -1)

        require.Equal(t, len(fg.Names), len(fg.MappingNames))
        for i, name := range fg.Names {
            if name == "total" {
                require.Equal(t, "", fg.MappingNames[i])
                continue
            }
            require.Equal(t, nameToMapping[name], fg.MappingNames[i])
        }
    })

    t.Run("empty mapping returns empty strings", func(t *testing.T) {
        s := new(FunctionNameTree)
        s.InsertStack(1, "foo", "bar")

        fg := NewFlameGraph(s, emptyMapping(), -1)

        require.Equal(t, len(fg.Names), len(fg.MappingNames))
        for _, m := range fg.MappingNames {
            require.Equal(t, "", m)
        }
    })

    t.Run("partial mapping leaves missing names as empty string", func(t *testing.T) {
        s := new(FunctionNameTree)
        s.InsertStack(1, "foo", "bar")

        nameToMapping := map[string]string{
            "foo": "foo_file",
        }

        fg := NewFlameGraph(s, nameToMapping, -1)

        require.Equal(t, len(fg.Names), len(fg.MappingNames))
        for i, name := range fg.Names {
            if name == "bar" {
                require.Equal(t, "", fg.MappingNames[i])
            }
        }
    })
}

func Test_FlameGraphMerger_Mapping(t *testing.T) {
    t.Run("merge mapping stores values", func(t *testing.T) {
        m := NewFlameGraphMerger()
        src := map[string]string{
            "foo": "foo_file",
            "bar": "bar_file",
        }
        m.MergeMapping(src)
        require.Equal(t, src, m.Mapping())
    })
}