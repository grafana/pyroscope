package pprof

import (
	"testing"

	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

func TestNormalizeProfile(t *testing.T) {
	p := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 2, Unit: 1},
			{Type: 3, Unit: 4},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 3}, Value: []int64{0, 1}, Label: []*profilev1.Label{{Num: 10, Key: 1}, {Num: 11, Key: 1}}},
			// Those samples should be dropped.
			{LocationId: []uint64{1, 2, 3}, Value: []int64{0, 0}, Label: []*profilev1.Label{{Num: 10, Key: 1}}},
			{LocationId: []uint64{4}, Value: []int64{0, 0}, Label: []*profilev1.Label{{Num: 10, Key: 1}}},
		},
		Mapping: []*profilev1.Mapping{},
		Location: []*profilev1.Location{
			{Id: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 1}, {FunctionId: 2, Line: 3}}},
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}, {FunctionId: 3, Line: 3}}},
			{Id: 3, Line: []*profilev1.Line{{FunctionId: 3, Line: 1}, {FunctionId: 4, Line: 3}}},
			{Id: 4, Line: []*profilev1.Line{{FunctionId: 5, Line: 1}}},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 5, SystemName: 6, Filename: 7, StartLine: 1},
			{Id: 2, Name: 8, SystemName: 9, Filename: 10, StartLine: 1},
			{Id: 3, Name: 11, SystemName: 12, Filename: 13, StartLine: 1},
			{Id: 4, Name: 14, SystemName: 15, Filename: 7, StartLine: 1},
			{Id: 5, Name: 16, SystemName: 17, Filename: 18, StartLine: 1},
		},
		StringTable: []string{
			"memory", "bytes", "in_used", "allocs", "count",
			"main", "runtime.main", "main.go", // fn1
			"foo", "runtime.foo", "foo.go", // fn2
			"bar", "runtime.bar", "bar.go", // fn3
			"buzz", "runtime.buzz", // fn4
			"bla", "runtime.bla", "bla.go", // fn5
		},
		PeriodType:        &profilev1.ValueType{Type: 0, Unit: 1},
		Comment:           []int64{},
		DefaultSampleType: 0,
	}

	pf := &Profile{Profile: p}
	pf.Normalize()
	require.Equal(t, pf.Profile, &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 2, Unit: 1},
			{Type: 3, Unit: 4},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 3}, Value: []int64{0, 1}, Label: []*profilev1.Label{}},
		},
		Mapping: []*profilev1.Mapping{},
		Location: []*profilev1.Location{
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}, {FunctionId: 3, Line: 3}}},
			{Id: 3, Line: []*profilev1.Line{{FunctionId: 3, Line: 1}, {FunctionId: 4, Line: 3}}},
		},
		Function: []*profilev1.Function{
			{Id: 2, Name: 6, SystemName: 7, Filename: 8, StartLine: 1},
			{Id: 3, Name: 9, SystemName: 10, Filename: 11, StartLine: 1},
			{Id: 4, Name: 12, SystemName: 13, Filename: 5, StartLine: 1},
		},
		StringTable: []string{
			"memory", "bytes", "in_used", "allocs", "count",
			"main.go",
			"foo", "runtime.foo", "foo.go",
			"bar", "runtime.bar", "bar.go",
			"buzz", "runtime.buzz",
		},
		PeriodType:        &profilev1.ValueType{Type: 0, Unit: 1},
		Comment:           []int64{},
		DefaultSampleType: 0,
	})
}

func TestRemoveDuplicateSampleStacktraces(t *testing.T) {
	p, err := OpenFile("testdata/heap")
	require.NoError(t, err)
	duplicate := countSampleDuplicates(p)
	total := len(p.Sample)
	t.Log("total dupe", duplicate)
	t.Log("total samples", total)

	p.Normalize()

	require.Equal(t, 0, countSampleDuplicates(p), "duplicates should be removed")
	require.Equal(t, total-duplicate, len(p.Sample), "unexpected total samples")
}

func countSampleDuplicates(p *Profile) int {
	hashes := p.hasher.Hashes(p.Sample)
	uniq := map[uint64][]*profilev1.Sample{}
	for i, s := range p.Sample {

		if _, ok := uniq[hashes[i]]; !ok {
			uniq[hashes[i]] = []*profilev1.Sample{s}
			continue
		}
		uniq[hashes[i]] = append(uniq[hashes[i]], s)
	}
	totalDupe := 0
	for _, v := range uniq {
		totalDupe += len(v) - 1
	}
	return totalDupe
}
