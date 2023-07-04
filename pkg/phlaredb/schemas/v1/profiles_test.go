package v1

import (
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"

	phlareparquet "github.com/grafana/phlare/pkg/parquet"
)

func TestInMemoryProfilesRowReader(t *testing.T) {
	r := NewProfilesRowReader(
		generateProfiles(10),
	)

	batch := make([]parquet.Row, 3)
	count := 0
	for {
		n, err := r.ReadRows(batch)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		count += n
		if n == 0 || err == io.EOF {
			break
		}
	}
	require.Equal(t, 10, count)
}

const samplesPerProfile = 100

func TestRoundtripProfile(t *testing.T) {
	profiles := generateProfiles(1000)
	iprofiles := generateMemoryProfiles(1000)
	actual, err := phlareparquet.ReadAll(NewInMemoryProfilesRowReader(iprofiles))
	require.NoError(t, err)
	expected, err := phlareparquet.ReadAll(NewProfilesRowReader(profiles))
	require.NoError(t, err)
	require.Equal(t, expected, actual)

	t.Run("EmptyOptionalField", func(t *testing.T) {
		profiles := generateProfiles(1)
		for _, p := range profiles {
			p.DurationNanos = 0
			p.Period = 0
			p.DefaultSampleType = 0
			p.KeepFrames = 0
		}
		inMemoryProfiles := generateMemoryProfiles(1)
		for i := range inMemoryProfiles {
			inMemoryProfiles[i].DurationNanos = 0
			inMemoryProfiles[i].Period = 0
			inMemoryProfiles[i].DefaultSampleType = 0
			inMemoryProfiles[i].KeepFrames = 0
		}
		expected, err := phlareparquet.ReadAll(NewProfilesRowReader(profiles))
		require.NoError(t, err)
		actual, err := phlareparquet.ReadAll(NewInMemoryProfilesRowReader(inMemoryProfiles))
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})
	t.Run("EmptyComment", func(t *testing.T) {
		profiles := generateProfiles(1)
		for _, p := range profiles {
			p.Comments = nil
		}
		inMemoryProfiles := generateMemoryProfiles(1)
		for i := range inMemoryProfiles {
			inMemoryProfiles[i].Comments = nil
		}
		expected, err := phlareparquet.ReadAll(NewProfilesRowReader(profiles))
		require.NoError(t, err)
		actual, err := phlareparquet.ReadAll(NewInMemoryProfilesRowReader(inMemoryProfiles))
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})

	t.Run("EmptySamples", func(t *testing.T) {
		profiles := generateProfiles(1)
		for _, p := range profiles {
			p.Samples = nil
		}
		inMemoryProfiles := generateMemoryProfiles(1)
		for i := range inMemoryProfiles {
			inMemoryProfiles[i].Samples = Samples{}
		}
		expected, err := phlareparquet.ReadAll(NewProfilesRowReader(profiles))
		require.NoError(t, err)
		actual, err := phlareparquet.ReadAll(NewInMemoryProfilesRowReader(inMemoryProfiles))
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})
}

func TestCompactSamples(t *testing.T) {
	require.Equal(t, Samples{
		StacktraceIDs: []uint32{1, 2, 3, 2, 5, 1, 7, 7, 1},
		Values:        []uint64{1, 1, 1, 1, 1, 3, 1, 0, 1},
	}.Compact(true), Samples{
		StacktraceIDs: []uint32{1, 2, 3, 5, 7},
		Values:        []uint64{5, 2, 1, 1, 1},
	})

	require.Equal(t, Samples{
		StacktraceIDs: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9},
		Values:        []uint64{1, 0, 1, 1, 1, 0, 1, 1, 0},
	}.Compact(false), Samples{
		StacktraceIDs: []uint32{1, 3, 4, 5, 7, 8},
		Values:        []uint64{1, 1, 1, 1, 1, 1},
	})

	require.Equal(t, Samples{
		StacktraceIDs: []uint32{1, 2, 3},
		Values:        []uint64{1, 2, 3},
	}.Compact(false), Samples{
		StacktraceIDs: []uint32{1, 2, 3},
		Values:        []uint64{1, 2, 3},
	})
}

func BenchmarkRowReader(b *testing.B) {
	profiles := generateProfiles(1000)
	iprofiles := generateMemoryProfiles(1000)
	b.Run("in-memory", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := phlareparquet.ReadAll(NewInMemoryProfilesRowReader(iprofiles))
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("schema", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := phlareparquet.ReadAll(NewProfilesRowReader(profiles))
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestMergeProfiles(t *testing.T) {
	reader := NewMergeProfilesRowReader([]parquet.RowReader{
		NewInMemoryProfilesRowReader([]InMemoryProfile{
			{SeriesIndex: 1, TimeNanos: 1},
			{SeriesIndex: 2, TimeNanos: 2},
			{SeriesIndex: 3, TimeNanos: 3},
		}),
		NewInMemoryProfilesRowReader([]InMemoryProfile{
			{SeriesIndex: 1, TimeNanos: 4},
			{SeriesIndex: 2, TimeNanos: 5},
			{SeriesIndex: 3, TimeNanos: 6},
		}),
		NewInMemoryProfilesRowReader([]InMemoryProfile{
			{SeriesIndex: 1, TimeNanos: 7},
			{SeriesIndex: 2, TimeNanos: 8},
			{SeriesIndex: 3, TimeNanos: 9},
		}),
	})

	actual, err := phlareparquet.ReadAll(reader)
	require.NoError(t, err)
	compareProfileRows(t, generateProfileRow([]InMemoryProfile{
		{SeriesIndex: 1, TimeNanos: 1},
		{SeriesIndex: 1, TimeNanos: 4},
		{SeriesIndex: 1, TimeNanos: 7},
		{SeriesIndex: 2, TimeNanos: 2},
		{SeriesIndex: 2, TimeNanos: 5},
		{SeriesIndex: 2, TimeNanos: 8},
		{SeriesIndex: 3, TimeNanos: 3},
		{SeriesIndex: 3, TimeNanos: 6},
		{SeriesIndex: 3, TimeNanos: 9},
	}), actual)
}

func TestLessProfileRows(t *testing.T) {
	for _, tc := range []struct {
		a, b     parquet.Row
		expected bool
	}{
		{
			a:        generateProfileRow([]InMemoryProfile{{SeriesIndex: 1, TimeNanos: 1}})[0],
			b:        generateProfileRow([]InMemoryProfile{{SeriesIndex: 1, TimeNanos: 1}})[0],
			expected: false,
		},
		{
			a:        generateProfileRow([]InMemoryProfile{{SeriesIndex: 1, TimeNanos: 1}})[0],
			b:        generateProfileRow([]InMemoryProfile{{SeriesIndex: 1, TimeNanos: 2}})[0],
			expected: true,
		},
		{
			a:        generateProfileRow([]InMemoryProfile{{SeriesIndex: 1, TimeNanos: 1}})[0],
			b:        generateProfileRow([]InMemoryProfile{{SeriesIndex: 2, TimeNanos: 1}})[0],
			expected: true,
		},
	} {
		t.Run("", func(t *testing.T) {
			require.Equal(t, tc.expected, lessProfileRows(tc.a, tc.b))
		})
	}
}

func BenchmarkProfileRows(b *testing.B) {
	a := generateProfileRow([]InMemoryProfile{{SeriesIndex: 1, TimeNanos: 1}})[0]
	a1 := generateProfileRow([]InMemoryProfile{{SeriesIndex: 1, TimeNanos: 2}})[0]
	a2 := generateProfileRow([]InMemoryProfile{{SeriesIndex: 2, TimeNanos: 1}})[0]

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		lessProfileRows(a, a)
		lessProfileRows(a, a1)
		lessProfileRows(a, a2)
	}
}

func compareProfileRows(t *testing.T, expected, actual []parquet.Row) {
	t.Helper()
	require.Equal(t, len(expected), len(actual))
	for i := range expected {
		expectedProfile, actualProfile := &Profile{}, &Profile{}
		require.NoError(t, profilesSchema.Reconstruct(actualProfile, actual[i]))
		require.NoError(t, profilesSchema.Reconstruct(expectedProfile, expected[i]))
		require.Equal(t, expectedProfile, actualProfile, "row %d", i)
	}
}

func generateProfileRow(in []InMemoryProfile) []parquet.Row {
	rows := make([]parquet.Row, len(in))
	for i, p := range in {
		rows[i] = deconstructMemoryProfile(p, rows[i])
	}
	return rows
}

func generateMemoryProfiles(n int) []InMemoryProfile {
	profiles := make([]InMemoryProfile, n)
	for i := 0; i < n; i++ {
		stacktraceID := make([]uint32, samplesPerProfile)
		value := make([]uint64, samplesPerProfile)
		for j := 0; j < samplesPerProfile; j++ {
			stacktraceID[j] = uint32(j)
			value[j] = uint64(j)
		}
		profiles[i] = InMemoryProfile{
			ID:                uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i)),
			SeriesIndex:       uint32(i),
			DropFrames:        1,
			KeepFrames:        3,
			TimeNanos:         int64(i),
			TotalValue:        100,
			Period:            100000,
			DurationNanos:     1000000000,
			Comments:          []int64{1, 2, 3},
			DefaultSampleType: 2,
			Samples: Samples{
				StacktraceIDs: stacktraceID,
				Values:        value,
			},
		}
	}
	return profiles
}

func generateProfiles(n int) []*Profile {
	profiles := make([]*Profile, n)
	for i := 0; i < n; i++ {
		profiles[i] = &Profile{
			ID:                uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i)),
			SeriesIndex:       uint32(i),
			DropFrames:        1,
			KeepFrames:        3,
			TotalValue:        100,
			TimeNanos:         int64(i),
			Period:            100000,
			DurationNanos:     1000000000,
			Comments:          []int64{1, 2, 3},
			DefaultSampleType: 2,
			Samples:           generateSamples(samplesPerProfile),
		}
	}

	return profiles
}

func generateSamples(n int) []*Sample {
	samples := make([]*Sample, n)
	for i := 0; i < n; i++ {
		samples[i] = &Sample{
			StacktraceID: uint64(i),
			Value:        int64(i),
		}
	}
	return samples
}
