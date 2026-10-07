package symdb

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

func Test_lookupTable(t *testing.T) {
	// Given the source data set.
	// Copy arbitrary subsets of items from src to dst.
	var dst []string
	src := []string{
		"zero",
		"one",
		"two",
		"three",
		"four",
		"five",
		"six",
		"seven",
	}

	type testCase struct {
		description string
		input       []uint32
		expected    []string
	}

	testCases := []testCase{
		{
			description: "empty table",
			input:       []uint32{5, 0, 3, 1, 2, 2, 4},
			expected:    []string{"five", "zero", "three", "one", "two", "two", "four"},
		},
		{
			description: "no new values",
			input:       []uint32{2, 1, 2, 3},
			expected:    []string{"two", "one", "two", "three"},
		},
		{
			description: "new value mixed",
			input:       []uint32{2, 1, 6, 2, 3},
			expected:    []string{"two", "one", "six", "two", "three"},
		},
	}

	// Try to lookup values in src lazily.
	// Table size must be greater or equal
	// to the source data set.
	l := newLookupTable[string](10)

	populate := func(t *testing.T, x []uint32) {
		for i, v := range x {
			x[i] = l.tryLookup(v)
		}
		// Resolve unknown yet values.
		// Mind the order and deduplication.
		p := -1
		for it := l.iter(); it.Err() == nil && it.Next(); {
			m := int(it.At())
			if m <= p {
				t.Fatal("iterator order invalid")
			}
			p = m
			it.setValue(src[m])
		}
	}

	resolveAppend := func() {
		// Populate dst with the newly resolved values.
		// Note that order in dst does not have to match src.
		for i, v := range l.values {
			l.storeResolved(i, uint32(len(dst)))
			dst = append(dst, v)
		}
	}

	resolve := func(x []uint32) []string {
		// Lookup resolved values.
		var resolved []string
		for _, v := range x {
			resolved = append(resolved, dst[l.lookupResolved(v)])
		}
		return resolved
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			l.reset()
			populate(t, tc.input)
			resolveAppend()
			assert.Equal(t, tc.expected, resolve(tc.input))
		})
	}

	assert.Len(t, dst, 7)
	assert.NotContains(t, dst, "seven")
}

// Location addresses must survive the rewrite: a partition with an empty
// functions table — a native profile that has not been symbolized, where
// every location is line-less — used to have all its locations rewritten
// as zero values, permanently losing the addresses at compaction.
func Test_Rewriter_location_addresses(t *testing.T) {
	for _, tc := range []struct {
		description string
		profile     *profilev1.Profile
		expected    []uint64
	}{
		{
			description: "partition without functions",
			profile:     linelessLocationsProfile(),
			expected:    []uint64{0x1500, 0x3c5a},
		},
		{
			description: "line-less location in a partition with functions",
			profile:     mixedLocationsProfile(),
			expected:    []uint64{0x3c5a},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			src := newMemSuite(t, nil)
			indexed := src.db.WriteProfileSymbols(0, tc.profile)
			require.NotEmpty(t, indexed)

			dst := NewSymDB(nil)
			rw := NewRewriter(dst, src.db, nil)
			for _, p := range indexed {
				ids := slices.Clone(p.Samples.StacktraceIDs)
				require.NoError(t, rw.Rewrite(0, ids))
			}

			pr, err := dst.Partition(context.Background(), 0)
			require.NoError(t, err)
			addresses := make([]uint64, 0, len(pr.Symbols().Locations))
			for _, loc := range pr.Symbols().Locations {
				addresses = append(addresses, loc.Address)
			}
			for _, addr := range tc.expected {
				assert.Contains(t, addresses, addr)
			}
		})
	}
}

func linelessLocationsProfile() *profilev1.Profile {
	return &profilev1.Profile{
		StringTable: []string{"", "libfoo.so", "build-id-f00"},
		Mapping: []*profilev1.Mapping{
			{Id: 1, MemoryLimit: 0x1000000, Filename: 1, BuildId: 2},
		},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Address: 0x1500},
			{Id: 2, MappingId: 1, Address: 0x3c5a},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1}, Value: []int64{100}},
			{LocationId: []uint64{2}, Value: []int64{200}},
			{LocationId: []uint64{1, 2}, Value: []int64{3}},
		},
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}
}

func mixedLocationsProfile() *profilev1.Profile {
	return &profilev1.Profile{
		StringTable: []string{"", "libfoo.so", "build-id-f00", "main", "main.go"},
		Mapping: []*profilev1.Mapping{
			{Id: 1, MemoryLimit: 0x1000000, Filename: 1, BuildId: 2},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 3, Filename: 4},
		},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 5}}},
			{Id: 2, MappingId: 1, Address: 0x3c5a},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 1}, Value: []int64{77}},
		},
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}
}
