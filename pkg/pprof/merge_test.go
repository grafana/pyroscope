package pprof

import (
	"testing"

	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

func Test_Merge_Single(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var m ProfileMerge
	require.NoError(t, m.Merge(p.Profile))
	testhelper.EqualProto(t, p.Profile, m.Profile())
}

func Test_Merge_Self(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var m ProfileMerge
	require.NoError(t, m.Merge(p.Profile))
	require.NoError(t, m.Merge(p.Profile))
	for i := range p.Sample {
		s := p.Sample[i]
		for j := range s.Value {
			s.Value[j] *= 2
		}
	}
	p.DurationNanos *= 2
	testhelper.EqualProto(t, p.Profile, m.Profile())
}

func Test_Merge_Halves(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)

	a := p.Profile.CloneVT()
	b := p.Profile.CloneVT()
	n := len(p.Sample) / 2
	a.Sample = a.Sample[:n]
	b.Sample = b.Sample[n:]

	var m ProfileMerge
	require.NoError(t, m.Merge(a))
	require.NoError(t, m.Merge(b))

	// Merge with self for normalisation.
	var sm ProfileMerge
	require.NoError(t, sm.Merge(p.Profile))
	p.DurationNanos *= 2
	testhelper.EqualProto(t, p.Profile, m.Profile())
}

func Test_Merge_Sample(t *testing.T) {
	stringTable := []string{
		"",
		"samples",
		"count",
		"cpu",
		"nanoseconds",
		"foo",
		"bar",
		"profile_id",
		"c717c11b87121639",
		"function",
		"slow",
		"8c946fa4ae322f7f",
		"fast",
		"main.work",
		"/Users/kolesnikovae/Documents/src/pyroscope/examples/golang-push/simple/main.go",
		"main.slowFunction.func1",
		"runtime/pprof.Do",
		"/usr/local/go/src/runtime/pprof/runtime.go",
		"main.slowFunction",
		"main.main.func2",
		"github.com/pyroscope-io/client/pyroscope.TagWrapper.func1",
		"/Users/kolesnikovae/go/pkg/mod/github.com/pyroscope-io/client@v0.2.4-0.20220607180407-0ba26860ce5b/pyroscope/api.go",
		"github.com/pyroscope-io/client/pyroscope.TagWrapper",
		"main.main",
		"runtime.main",
		"/usr/local/go/src/runtime/proc.go",
		"main.fastFunction.func1",
		"main.fastFunction",
	}

	a := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 1,
				Unit: 2,
			},
			{
				Type: 3,
				Unit: 4,
			},
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1, 2, 3},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
					{Key: 9, Str: 10},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:           1,
				HasFunctions: true,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   19497668,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 19}},
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   19498429,
				Line:      []*profilev1.Line{{FunctionId: 2, Line: 43}},
			},
			{
				Id:        3,
				MappingId: 1,
				Address:   19267106,
				Line:      []*profilev1.Line{{FunctionId: 3, Line: 40}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:         1,
				Name:       13,
				SystemName: 13,
				Filename:   14,
			},
			{
				Id:         2,
				Name:       15,
				SystemName: 15,
				Filename:   14,
			},
			{
				Id:         3,
				Name:       16,
				SystemName: 16,
				Filename:   17,
			},
		},
		StringTable:   stringTable,
		TimeNanos:     1654798932062349000,
		DurationNanos: 10123363553,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}

	b := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 1,
				Unit: 2,
			},
			{
				Type: 3,
				Unit: 4,
			},
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 11},
					{Key: 9, Str: 12},
				},
			},
			{
				LocationId: []uint64{2, 3, 4}, // Same
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
					{Key: 9, Str: 10},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:           1,
				HasFunctions: true,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   19499013,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 42}},
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   19497668,
				Line:      []*profilev1.Line{{FunctionId: 2, Line: 19}},
			},
			{
				Id:        3,
				MappingId: 1,
				Address:   19498429,
				Line:      []*profilev1.Line{{FunctionId: 3, Line: 43}},
			},
			{
				Id:        4,
				MappingId: 1,
				Address:   19267106,
				Line:      []*profilev1.Line{{FunctionId: 4, Line: 40}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:         1,
				Name:       18,
				SystemName: 18,
				Filename:   14,
			},
			{
				Id:         2,
				Name:       13,
				SystemName: 13,
				Filename:   14,
			},
			{
				Id:         3,
				Name:       15,
				SystemName: 15,
				Filename:   14,
			},
			{
				Id:         4,
				Name:       16,
				SystemName: 16,
				Filename:   17,
			},
		},
		StringTable:   stringTable,
		TimeNanos:     1654798932062349000,
		DurationNanos: 10123363553,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}

	expected := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 1,
				Unit: 2,
			},
			{
				Type: 3,
				Unit: 4,
			},
		},
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1, 2, 3},
				Value:      []int64{2, 20000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
					{Key: 9, Str: 10},
				},
			},
			{
				LocationId: []uint64{4},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 11},
					{Key: 9, Str: 12},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{
				Id:           1,
				HasFunctions: true,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   19497668,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 19}},
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   19498429,
				Line:      []*profilev1.Line{{FunctionId: 2, Line: 43}},
			},
			{
				Id:        3,
				MappingId: 1,
				Address:   19267106,
				Line:      []*profilev1.Line{{FunctionId: 3, Line: 40}},
			},
			{
				Id:        4,
				MappingId: 1,
				Address:   19499013,
				Line:      []*profilev1.Line{{FunctionId: 4, Line: 42}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:         1,
				Name:       13,
				SystemName: 13,
				Filename:   14,
			},
			{
				Id:         2,
				Name:       15,
				SystemName: 15,
				Filename:   14,
			},
			{
				Id:         3,
				Name:       16,
				SystemName: 16,
				Filename:   17,
			},
			{
				Id:         4,
				Name:       18,
				SystemName: 18,
				Filename:   14,
			},
		},
		StringTable:   stringTable,
		TimeNanos:     1654798932062349000,
		DurationNanos: 20246727106,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}

	var m ProfileMerge
	require.NoError(t, m.Merge(a))
	require.NoError(t, m.Merge(b))

	testhelper.EqualProto(t, expected, m.Profile())
}

func TestMergeEmpty(t *testing.T) {
	var m ProfileMerge

	err := m.Merge(&profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{
				Type: 2,
				Unit: 1,
			},
		},
		PeriodType: &profilev1.ValueType{
			Type: 2,
			Unit: 1,
		},
		StringTable: []string{"", "nanoseconds", "cpu"},
	})
	require.NoError(t, err)
	err = m.Merge(&profilev1.Profile{
		Sample: []*profilev1.Sample{
			{
				LocationId: []uint64{1},
				Value:      []int64{1},
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Line:      []*profilev1.Line{{FunctionId: 1, Line: 1}},
			},
		},
		Function: []*profilev1.Function{
			{
				Id:   1,
				Name: 1,
			},
		},
		SampleType: []*profilev1.ValueType{
			{
				Type: 3,
				Unit: 2,
			},
		},
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 2,
		},
		Mapping: []*profilev1.Mapping{
			{
				Id: 1,
			},
		},
		StringTable: []string{"", "bar", "nanoseconds", "cpu"},
	})
	require.NoError(t, err)
}
