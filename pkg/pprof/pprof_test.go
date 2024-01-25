package pprof

import (
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestNormalizeProfile(t *testing.T) {
	currentTime = func() time.Time {
		t, _ := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
		return t
	}
	defer func() {
		currentTime = time.Now
	}()

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
		Mapping: []*profilev1.Mapping{{Id: 1, HasFunctions: true, MemoryStart: 100, MemoryLimit: 200, FileOffset: 200}},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Address: 5, Line: []*profilev1.Line{{FunctionId: 1, Line: 1}, {FunctionId: 2, Line: 3}}},
			{Id: 2, MappingId: 1, Address: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}, {FunctionId: 3, Line: 3}}},
			{Id: 3, MappingId: 1, Address: 1, Line: []*profilev1.Line{{FunctionId: 3, Line: 1}, {FunctionId: 4, Line: 3}}},
			{Id: 4, MappingId: 1, Address: 0, Line: []*profilev1.Line{{FunctionId: 5, Line: 1}}},
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
		Mapping: []*profilev1.Mapping{{
			Id:           1,
			HasFunctions: true,
		}},
		Location: []*profilev1.Location{
			{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}, {FunctionId: 3, Line: 3}}},
			{Id: 3, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 3, Line: 1}, {FunctionId: 4, Line: 3}}},
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
		TimeNanos:         1577836800000000000,
		DefaultSampleType: 0,
	})
}

func TestNormalizeProfile_SampleLabels(t *testing.T) {
	currentTime = func() time.Time {
		t, _ := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
		return t
	}
	defer func() {
		currentTime = time.Now
	}()

	p := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 1}, Value: []int64{10}, Label: []*profilev1.Label{{Str: 10, Key: 1}, {Str: 11, Key: 2}}},
			{LocationId: []uint64{2, 1}, Value: []int64{10}, Label: []*profilev1.Label{{Str: 12, Key: 2}, {Str: 10, Key: 1}}},
			{LocationId: []uint64{2, 1}, Value: []int64{10}, Label: []*profilev1.Label{{Str: 11, Key: 2}, {Str: 10, Key: 1}}},
		},
		Mapping: []*profilev1.Mapping{{Id: 1, HasFunctions: true, MemoryStart: 100, MemoryLimit: 200, FileOffset: 200}},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Address: 5, Line: []*profilev1.Line{{FunctionId: 1, Line: 1}}},
			{Id: 2, MappingId: 1, Address: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}}},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 3, SystemName: 3, Filename: 4, StartLine: 1},
			{Id: 2, Name: 5, SystemName: 5, Filename: 4, StartLine: 1},
		},
		StringTable: []string{
			"",
			"cpu", "nanoseconds",
			"main", "main.go",
			"foo",
		},
		PeriodType: &profilev1.ValueType{Type: 1, Unit: 2},
	}

	pf := &Profile{Profile: p}
	pf.Normalize()
	require.Equal(t, pf.Profile, &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 1}, Value: []int64{10}, Label: []*profilev1.Label{{Str: 10, Key: 1}, {Str: 12, Key: 2}}},
			{LocationId: []uint64{2, 1}, Value: []int64{20}, Label: []*profilev1.Label{{Str: 10, Key: 1}, {Str: 11, Key: 2}}},
		},
		Mapping: []*profilev1.Mapping{{Id: 1, HasFunctions: true}},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 1}}},
			{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2, Line: 1}}},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 3, SystemName: 3, Filename: 4, StartLine: 1},
			{Id: 2, Name: 5, SystemName: 5, Filename: 4, StartLine: 1},
		},
		StringTable: []string{
			"",
			"cpu", "nanoseconds",
			"main", "main.go",
			"foo",
		},
		PeriodType: &profilev1.ValueType{Type: 1, Unit: 2},
		TimeNanos:  1577836800000000000,
	})
}

func TestFromProfile(t *testing.T) {
	out, err := FromProfile(testhelper.FooBarProfile)
	require.NoError(t, err)
	data, err := proto.Marshal(out)
	require.NoError(t, err)
	outProfile, err := profile.ParseUncompressed(data)
	require.NoError(t, err)

	require.Equal(t, testhelper.FooBarProfile, outProfile)
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func BenchmarkNormalize(b *testing.B) {
	profiles := make([]*Profile, b.N)
	for i := 0; i < b.N; i++ {
		builder := testhelper.NewProfileBuilder(0).CPUProfile()
		// 10% of samples should be dropped.
		for i := 0; i < 1000; i++ {
			builder.ForStacktraceString(RandStringBytes(3), RandStringBytes(3)).AddSamples(0)
		}
		for i := 0; i < 10000; i++ {
			builder.ForStacktraceString(RandStringBytes(3), RandStringBytes(3)).AddSamples(1)
		}
		profiles[i] = &Profile{Profile: builder.Profile}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		profiles[i].Normalize()
	}
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

func TestEmptyMappingJava(t *testing.T) {
	p, err := OpenFile("testdata/profile_java")
	require.NoError(t, err)
	require.Len(t, p.Mapping, 0)

	p.Normalize()
	require.Len(t, p.Mapping, 1)

	for _, loc := range p.Location {
		require.Equal(t, loc.MappingId, uint64(1))
	}
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

var prof *profilev1.Profile

func BenchmarkFromRawBytes(b *testing.B) {
	data, err := os.ReadFile("testdata/heap")
	require.NoError(b, err)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err := FromBytes(data, func(p *profilev1.Profile, i int) error {
			prof = p
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Test_GroupSamplesByLabels(t *testing.T) {
	type testCase struct {
		description string
		input       *profilev1.Profile
		expected    []SampleGroup
	}

	testCases := []*testCase{
		{
			description: "no samples",
			input:       new(profilev1.Profile),
			expected:    nil,
		},
		{
			description: "single label set",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
				},
			},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					},
				},
			},
		},
		{
			description: "all sets are unique",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
				},
			},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
					},
				},
			},
		},
		{
			description: "ends with unique label set",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
				},
			},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
						{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
					},
				},
			},
		},
		{
			description: "starts with unique label set",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
				},
			},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
						{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
					},
				},
			},
		},
		{
			description: "no unique sets",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
				},
			},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
						{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
						{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			require.Equal(t, tc.expected, GroupSamplesByLabels(tc.input))
		})
	}
}

func Test_FilterLabelsInPlace(t *testing.T) {
	type testCase struct {
		labels        []*profilev1.Label
		keys          []int64
		expectedOrder []*profilev1.Label
		expectedIndex int
	}

	testCases := []testCase{
		{
			labels: []*profilev1.Label{
				{Key: 1, Str: 100},
				{Key: 2, Str: 200},
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			keys: []int64{2, 4},
			expectedOrder: []*profilev1.Label{
				{Key: 2, Str: 200},
				{Key: 4, Str: 400},
				{Key: 3, Str: 300},
				{Key: 1, Str: 100},
				{Key: 5, Str: 500},
			},
			expectedIndex: 2,
		},
		{
			labels: []*profilev1.Label{
				{Key: 1, Str: 100},
				{Key: 2, Str: 200},
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			keys: []int64{1, 3, 5},
			expectedOrder: []*profilev1.Label{
				{Key: 1, Str: 100},
				{Key: 3, Str: 300},
				{Key: 5, Str: 500},
				{Key: 4, Str: 400},
				{Key: 2, Str: 200},
			},
			expectedIndex: 3,
		},
		{
			labels: []*profilev1.Label{
				{Key: 1, Str: 100},
				{Key: 2, Str: 200},
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			keys: []int64{6, 7},
			expectedOrder: []*profilev1.Label{
				{Key: 1, Str: 100},
				{Key: 2, Str: 200},
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			expectedIndex: 0,
		},
		{
			labels: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			keys: []int64{1, 2},
			expectedOrder: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			expectedIndex: 0,
		},
		{
			labels: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			keys: []int64{4},
			expectedOrder: []*profilev1.Label{
				{Key: 4, Str: 400},
				{Key: 3, Str: 300},
				{Key: 5, Str: 500},
			},
			expectedIndex: 1,
		},
		{
			labels: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			keys: []int64{3},
			expectedOrder: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			expectedIndex: 1,
		},
		{
			labels: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			keys: []int64{5},
			expectedOrder: []*profilev1.Label{
				{Key: 5, Str: 500},
				{Key: 4, Str: 400},
				{Key: 3, Str: 300},
			},
			expectedIndex: 1,
		},
		{
			labels: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			expectedOrder: []*profilev1.Label{
				{Key: 3, Str: 300},
				{Key: 4, Str: 400},
				{Key: 5, Str: 500},
			},
			expectedIndex: 0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run("", func(t *testing.T) {
			boundaryIdx := FilterLabelsInPlace(tc.labels, tc.keys)
			require.Equal(t, tc.expectedOrder, tc.labels)
			require.Equal(t, tc.expectedIndex, boundaryIdx)
		})
	}
}

func Test_GroupSamplesWithout(t *testing.T) {
	type testCase struct {
		description string
		input       *profilev1.Profile
		expected    []SampleGroup
		without     []int64
	}

	testCases := []*testCase{
		{
			description: "no samples",
			input:       new(profilev1.Profile),
			expected:    nil,
		},
		{
			description: "without all, single label set",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 2, Str: 2}, {Key: 1, Str: 1}}},
				},
			},
			without: []int64{1, 2},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 2, Str: 2}, {Key: 1, Str: 1}}},
					},
				},
			},
		},
		{
			description: "without all, many label sets",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}, {Key: 2, Str: 4}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}}},
					{Label: []*profilev1.Label{}},
				},
			},
			without: []int64{1, 2},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 2, Str: 2}, {Key: 1, Str: 1}}},
						{Label: []*profilev1.Label{{Key: 2, Str: 4}, {Key: 1, Str: 3}}},
						{Label: []*profilev1.Label{{Key: 1, Str: 3}}},
						{Label: []*profilev1.Label{}},
					},
				},
			},
		},
		{
			description: "without none",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 3}}},
					{Label: []*profilev1.Label{}},
				},
			},
			without: []int64{},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 3}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 2, Str: 2}, {Key: 1, Str: 1}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
						{Label: []*profilev1.Label{}},
					},
				},
			},
		},
		{
			description: "without single, multiple groups",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 100}, {Key: 3, Str: 3}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 101}, {Key: 3, Str: 3}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 102}, {Key: 3, Str: 4}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}}},
					{Label: []*profilev1.Label{}},
				},
			},
			without: []int64{2},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 3, Str: 3}, {Key: 1, Str: 1}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 2, Str: 100}}},
						{Label: []*profilev1.Label{{Key: 2, Str: 101}}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 3, Str: 4}, {Key: 1, Str: 1}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 2, Str: 102}}},
					},
				},
			},
		},
		{
			description: "without single, non-existent",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}, {Key: 3, Str: 3}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}}},
					{Label: []*profilev1.Label{}},
				},
			},
			without: []int64{7},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 3, Str: 3}, {Key: 2, Str: 2}, {Key: 1, Str: 1}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
			},
		},
		{
			description: "without multiple, non-existent mixed",
			input: &profilev1.Profile{
				Sample: []*profilev1.Sample{
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}, {Key: 3, Str: 3}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}, {Key: 3, Str: 13}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}, {Key: 3, Str: 3}, {Key: 5, Str: 5}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}, {Key: 2, Str: 2}, {Key: 3, Str: 13}, {Key: 5, Str: 15}}},
					{Label: []*profilev1.Label{{Key: 1, Str: 1}}},
					{Label: []*profilev1.Label{}},
				},
			},
			without: []int64{2, 3, 5},
			expected: []SampleGroup{
				{
					Labels: []*profilev1.Label{},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{}},
					},
				},
				{
					Labels: []*profilev1.Label{{Key: 1, Str: 1}},
					Samples: []*profilev1.Sample{
						{Label: []*profilev1.Label{{Key: 3, Str: 3}, {Key: 2, Str: 2}}},
						{Label: []*profilev1.Label{{Key: 3, Str: 13}, {Key: 2, Str: 2}}},
						{Label: []*profilev1.Label{{Key: 5, Str: 5}, {Key: 3, Str: 3}, {Key: 2, Str: 2}}},
						{Label: []*profilev1.Label{{Key: 5, Str: 15}, {Key: 3, Str: 13}, {Key: 2, Str: 2}}},
						{Label: []*profilev1.Label{}},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			require.Equal(t, tc.expected, GroupSamplesWithoutLabelsByKey(tc.input, tc.without))
		})
	}
}

func Test_SampleExporter_WholeProfile(t *testing.T) {
	p, err := OpenFile("testdata/heap")
	require.NoError(t, err)
	e := NewSampleExporter(p.Profile)
	n := new(profilev1.Profile)
	e.ExportSamples(n, p.Sample)

	// Samples are modified in-place, therefore
	// we have to re-read the profile.
	p, err = OpenFile("testdata/heap")
	require.NoError(t, err)
	requireProfilesEqual(t, p.Profile, n)
}

func requireProfilesEqual(t *testing.T, expected, actual *profilev1.Profile) {
	require.Equal(t, expected.SampleType, actual.SampleType)
	require.Equal(t, expected.PeriodType, actual.PeriodType)
	require.Equal(t, expected.Period, actual.Period)
	require.Equal(t, expected.Comment, actual.Comment)
	require.Equal(t, expected.DropFrames, actual.DropFrames)
	require.Equal(t, expected.KeepFrames, actual.KeepFrames)
	require.Equal(t, expected.DefaultSampleType, actual.DefaultSampleType)
	require.Equal(t, expected.TimeNanos, actual.TimeNanos)
	require.Equal(t, expected.DurationNanos, actual.DurationNanos)
	require.Equal(t, expected.Sample, actual.Sample)
	require.Equal(t, expected.Location, actual.Location)
	require.Equal(t, expected.Function, actual.Function)
	require.Equal(t, expected.Mapping, actual.Mapping)
	require.Equal(t, expected.StringTable, actual.StringTable)
}

func Test_SampleExporter_Partial(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	e := NewSampleExporter(p.Profile)
	n := new(profilev1.Profile)
	e.ExportSamples(n, p.Sample[:2])
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
				LocationId: []uint64{1, 2, 3, 4, 5, 6, 3, 7, 8, 9},
				Value:      []int64{1, 10000000},
				Label: []*profilev1.Label{
					{Key: 5, Str: 6},
					{Key: 7, Str: 8},
					{Key: 9, Str: 10},
				},
			},
			{
				LocationId: []uint64{1, 10, 6, 3, 7, 11, 12, 6, 3, 7, 8, 9},
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
			{
				Id:        5,
				MappingId: 1,
				Address:   19499251,
				Line:      []*profilev1.Line{{FunctionId: 5, Line: 68}},
			},
			{
				Id:        6,
				MappingId: 1,
				Address:   19285318,
				Line:      []*profilev1.Line{{FunctionId: 6, Line: 101}},
			},
			{
				Id:        7,
				MappingId: 1,
				Address:   19285188,
				Line:      []*profilev1.Line{{FunctionId: 7, Line: 101}},
			},
			{
				Id:        8,
				MappingId: 1,
				Address:   19499465,
				Line:      []*profilev1.Line{{FunctionId: 8, Line: 65}},
			},
			{
				Id:        9,
				MappingId: 1,
				Address:   17007057,
				Line:      []*profilev1.Line{{FunctionId: 9, Line: 250}},
			},
			{
				Id:        10,
				MappingId: 1,
				Address:   19497725,
				Line:      []*profilev1.Line{{FunctionId: 10, Line: 31}},
			},
			{
				Id:        11,
				MappingId: 1,
				Address:   19498309,
				Line:      []*profilev1.Line{{FunctionId: 11, Line: 30}},
			},
			{
				Id:        12,
				MappingId: 1,
				Address:   19499236,
				Line:      []*profilev1.Line{{FunctionId: 5, Line: 67}},
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
			{
				Id:         5,
				Name:       19,
				SystemName: 19,
				Filename:   14,
			},
			{
				Id:         6,
				Name:       20,
				SystemName: 20,
				Filename:   21,
			},
			{
				Id:         7,
				Name:       22,
				SystemName: 22,
				Filename:   21,
			},
			{
				Id:         8,
				Name:       23,
				SystemName: 23,
				Filename:   14,
			},
			{
				Id:         9,
				Name:       24,
				SystemName: 24,
				Filename:   25,
			},
			{
				Id:         10,
				Name:       26,
				SystemName: 26,
				Filename:   14,
			},
			{
				Id:         11,
				Name:       27,
				SystemName: 27,
				Filename:   14,
			},
		},
		StringTable: []string{
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
		},
		TimeNanos:     1654798932062349000,
		DurationNanos: 10123363553,
		PeriodType: &profilev1.ValueType{
			Type: 3,
			Unit: 4,
		},
		Period: 10000000,
	}
	requireProfilesEqual(t, expected, n)
}

func Test_GroupSamplesWithout_Go_CPU_profile(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)

	groups := GroupSamplesWithoutLabels(p.Profile, ProfileIDLabelName)
	require.Len(t, groups, 3)

	assert.Equal(t, groups[0].Labels, []*profilev1.Label{{Key: 18, Str: 19}})
	assert.Equal(t, len(groups[0].Samples), 5)

	assert.Equal(t, groups[1].Labels, []*profilev1.Label{{Key: 22, Str: 23}, {Key: 18, Str: 19}})
	assert.Equal(t, len(groups[1].Samples), 325)

	assert.Equal(t, groups[2].Labels, []*profilev1.Label{{Key: 22, Str: 27}, {Key: 18, Str: 19}})
	assert.Equal(t, len(groups[2].Samples), 150)
}

func Test_GroupSamplesWithout_dotnet_profile(t *testing.T) {
	p, err := OpenFile("testdata/dotnet.labels.pprof")
	require.NoError(t, err)

	groups := GroupSamplesWithoutLabels(p.Profile, ProfileIDLabelName)
	require.Len(t, groups, 1)
	assert.Equal(t, groups[0].Labels, []*profilev1.Label{{Key: 66, Str: 67}, {Key: 64, Str: 65}})
}

func Test_GetProfileLanguage_go_cpu_profile(t *testing.T) {
	p, err := OpenFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "go", language)
}

func Test_GetProfileLanguage_go_heap_profile(t *testing.T) {
	p, err := OpenFile("testdata/heap")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "go", language)
}

func Test_GetProfileLanguage_dotnet_profile(t *testing.T) {
	p, err := OpenFile("testdata/dotnet.labels.pprof")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "dotnet", language)
}

func Test_GetProfileLanguage_java_profile(t *testing.T) {
	p, err := OpenFile("testdata/profile_java")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "java", language)
}

func Test_GetProfileLanguage_python_profile(t *testing.T) {
	p, err := OpenFile("testdata/profile_python")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "python", language)
}

func Test_GetProfileLanguage_ruby_profile(t *testing.T) {
	p, err := OpenFile("testdata/profile_ruby")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "ruby", language)
}

func Test_GetProfileLanguage_nodejs_profile(t *testing.T) {
	p, err := OpenFile("testdata/profile_nodejs")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "nodejs", language)
}

func Test_GetProfileLanguage_rust_profile(t *testing.T) {
	p, err := OpenFile("testdata/profile_rust")
	require.NoError(t, err)

	language := GetLanguage(p, log.NewNopLogger())
	assert.Equal(t, "rust", language)
}

func Benchmark_GetProfileLanguage(b *testing.B) {
	tests := []string{
		"testdata/go.cpu.labels.pprof",
		"testdata/heap",
		"testdata/dotnet.labels.pprof",
		"testdata/profile_java",
		"testdata/profile_nodejs",
		"testdata/profile_python",
		"testdata/profile_ruby",
		"testdata/profile_rust",
	}

	for _, testdata := range tests {
		f := testdata
		b.Run(testdata, func(b *testing.B) {
			p, err := OpenFile(f)
			require.NoError(b, err)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				language := GetLanguage(p, log.NewNopLogger())
				if language == "unknown" {
					b.Fatal()
				}
			}
		})
	}
}
