package query

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

type makeTestIterFn func(pf *parquet.File, idx int, filter Predicate, selectAs string) Iterator

var iterTestCases = []struct {
	name     string
	makeIter makeTestIterFn
}{
	{"sync", func(pf *parquet.File, idx int, filter Predicate, selectAs string) Iterator {
		return NewSyncIterator(context.TODO(), pf.RowGroups(), idx, selectAs, 1000, filter, selectAs)
	}},
}

// TestNext compares the unrolled Next() with the original nextSlow() to
// prevent drift
func TestNext(t *testing.T) {
	rn1 := RowNumber{0, 0, 0, 0, 0, 0}
	rn2 := RowNumber{0, 0, 0, 0, 0, 0}

	for i := 0; i < 1000; i++ {
		r := rand.Intn(6)
		d := rand.Intn(6)

		rn1.Next(r, d)
		rn2.nextSlow(r, d)

		require.Equal(t, rn1, rn2)
	}
}

func TestRowNumber(t *testing.T) {
	tr := EmptyRowNumber()
	require.Equal(t, RowNumber{-1, -1, -1, -1, -1, -1}, tr)

	steps := []struct {
		repetitionLevel int
		definitionLevel int
		expected        RowNumber
	}{
		// Name.Language.Country examples from the Dremel whitepaper
		{0, 3, RowNumber{0, 0, 0, 0, -1, -1}},
		{2, 2, RowNumber{0, 0, 1, -1, -1, -1}},
		{1, 1, RowNumber{0, 1, -1, -1, -1, -1}},
		{1, 3, RowNumber{0, 2, 0, 0, -1, -1}},
		{0, 1, RowNumber{1, 0, -1, -1, -1, -1}},
	}

	for _, step := range steps {
		tr.Next(step.repetitionLevel, step.definitionLevel)
		require.Equal(t, step.expected, tr)
	}
}

func TestCompareRowNumbers(t *testing.T) {
	testCases := []struct {
		a, b     RowNumber
		expected int
	}{
		{RowNumber{-1}, RowNumber{0}, -1},
		{RowNumber{0}, RowNumber{0}, 0},
		{RowNumber{1}, RowNumber{0}, 1},

		{RowNumber{0, 1}, RowNumber{0, 2}, -1},
		{RowNumber{0, 2}, RowNumber{0, 1}, 1},
	}

	for _, tc := range testCases {
		require.Equal(t, tc.expected, CompareRowNumbers(MaxDefinitionLevel, tc.a, tc.b))
	}
}

func TestRowNumberPreceding(t *testing.T) {
	testCases := []struct {
		start, preceding RowNumber
	}{
		{RowNumber{1000, -1, -1, -1, -1, -1}, RowNumber{999, -1, -1, -1, -1, -1}},
		{RowNumber{1000, 0, 0, 0, 0, 0}, RowNumber{999, math.MaxInt64, math.MaxInt64, math.MaxInt64, math.MaxInt64, math.MaxInt64}},
	}

	for _, tc := range testCases {
		require.Equal(t, tc.preceding, tc.start.Preceding())
	}
}

func TestColumnIterator(t *testing.T) {
	for _, tc := range iterTestCases {
		t.Run(tc.name, func(t *testing.T) {
			testColumnIterator(t, tc.makeIter)
		})
	}
}

func testColumnIterator(t *testing.T, makeIter makeTestIterFn) {
	count := 100_000
	pf := createTestFile(t, count)

	idx, _ := GetColumnIndexByPath(pf, "A")
	iter := makeIter(pf, idx, nil, "A")
	defer iter.Close()

	for i := 0; i < count; i++ {
		require.True(t, iter.Next())
		res := iter.At()
		require.NotNil(t, res, "i=%d", i)
		require.Equal(t, RowNumber{int64(i), -1, -1, -1, -1, -1}, res.RowNumber)
		require.Equal(t, int64(i), res.ToMap()["A"][0].Int64())
	}

	require.False(t, iter.Next())
	require.NoError(t, iter.Err())
}

func TestColumnIteratorSeek(t *testing.T) {
	for _, tc := range iterTestCases {
		t.Run(tc.name, func(t *testing.T) {
			testColumnIteratorSeek(t, tc.makeIter)
		})
	}
}

func testColumnIteratorSeek(t *testing.T, makeIter makeTestIterFn) {
	count := 10_000
	pf := createTestFile(t, count)

	idx, _ := GetColumnIndexByPath(pf, "A")
	iter := makeIter(pf, idx, nil, "A")
	defer iter.Close()

	seekTos := []int64{
		100,
		1234,
		4567,
		5000,
		7890,
	}

	for _, seekTo := range seekTos {
		rn := EmptyRowNumber()
		rn[0] = seekTo
		require.True(t, iter.Seek(RowNumberWithDefinitionLevel{rn, 0}))
		res := iter.At()
		require.NotNil(t, res, "seekTo=%v", seekTo)
		require.Equal(t, RowNumber{seekTo, -1, -1, -1, -1, -1}, res.RowNumber)
		require.Equal(t, seekTo, res.ToMap()["A"][0].Int64())
	}
}

func TestColumnIteratorPredicate(t *testing.T) {
	for _, tc := range iterTestCases {
		t.Run(tc.name, func(t *testing.T) {
			testColumnIteratorPredicate(t, tc.makeIter)
		})
	}
}

func testColumnIteratorPredicate(t *testing.T, makeIter makeTestIterFn) {
	count := 10_000
	pf := createTestFile(t, count)

	pred := NewIntBetweenPredicate(7001, 7003)

	idx, _ := GetColumnIndexByPath(pf, "A")
	iter := makeIter(pf, idx, pred, "A")
	defer iter.Close()

	expectedResults := []int64{
		7001,
		7002,
		7003,
	}

	for _, expectedResult := range expectedResults {
		require.True(t, iter.Next())
		res := iter.At()
		require.NotNil(t, res)
		require.Equal(t, RowNumber{expectedResult, -1, -1, -1, -1, -1}, res.RowNumber)
		require.Equal(t, expectedResult, res.ToMap()["A"][0].Int64())
	}
}

func TestColumnIteratorExitEarly(t *testing.T) {
	type T struct{ A int }

	rows := []T{}
	count := 10_000
	for i := 0; i < count; i++ {
		rows = append(rows, T{i})
	}

	pf := createFileWith(t, rows, 2)
	idx, _ := GetColumnIndexByPath(pf, "A")
	readSize := 1000

	readIter := func(iter Iterator) (int, error) {
		received := 0
		for iter.Next() {
			received++
		}
		return received, iter.Err()
	}

	t.Run("cancelledEarly", func(t *testing.T) {
		// Cancel before iterating
		ctx, cancel := context.WithCancel(context.TODO())
		cancel()
		iter := NewSyncIterator(ctx, pf.RowGroups(), idx, "", readSize, nil, "A")
		count, err := readIter(iter)
		require.ErrorContains(t, err, "context canceled")
		require.Equal(t, 0, count)
	})

	t.Run("cancelledPartial", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.TODO())
		iter := NewSyncIterator(ctx, pf.RowGroups(), idx, "", readSize, nil, "A")

		// Read some results
		require.True(t, iter.Next())

		// Then cancel
		cancel()

		// Read again = context cancelled
		_, err := readIter(iter)
		require.ErrorContains(t, err, "context canceled")
	})

	t.Run("closedEarly", func(t *testing.T) {
		// Close before iterating
		iter := NewSyncIterator(context.TODO(), pf.RowGroups(), idx, "", readSize, nil, "A")
		iter.Close()
		count, err := readIter(iter)
		require.ErrorContains(t, err, "context canceled")
		require.Equal(t, 0, count)
	})

	t.Run("closedPartial", func(t *testing.T) {
		iter := NewSyncIterator(context.TODO(), pf.RowGroups(), idx, "", readSize, nil, "A")

		// Read some results
		require.True(t, iter.Next())

		// Then close
		iter.Close()

		// Read again = should close early
		res2, err := readIter(iter)
		require.ErrorContains(t, err, "context canceled")
		require.Less(t, readSize+res2, count)
	})
}

func BenchmarkColumnIterator(b *testing.B) {
	for _, tc := range iterTestCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkColumnIterator(b, tc.makeIter)
		})
	}
}

func benchmarkColumnIterator(b *testing.B, makeIter makeTestIterFn) {
	count := 100_000
	pf := createTestFile(b, count)

	idx, _ := GetColumnIndexByPath(pf, "A")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		iter := makeIter(pf, idx, nil, "A")
		actualCount := 0
		for iter.Next() {
			actualCount++
		}
		iter.Close()
		require.Equal(b, count, actualCount)
		//fmt.Println(actualCount)
	}
}

func createTestFile(t testing.TB, count int) *parquet.File {
	type T struct{ A int }

	rows := []T{}
	for i := 0; i < count; i++ {
		rows = append(rows, T{i})
	}

	pf := createFileWith(t, rows, 2)
	return pf
}

func createProfileLikeFile(t testing.TB, count int) *parquet.File {
	type T struct {
		SeriesID  uint32
		TimeNanos int64
	}

	// every row group is ordered by serieID and then time nanos
	// time is always increasing between rowgroups

	rowGroups := 10
	series := 8

	rows := make([]T, count)
	for i := range rows {

		rowsPerRowGroup := count / rowGroups
		seriesPerRowGroup := rowsPerRowGroup / series
		rowGroupNum := i / rowsPerRowGroup

		seriesID := uint32(i % (count / rowGroups) / (rowsPerRowGroup / series))
		rows[i] = T{
			SeriesID:  seriesID,
			TimeNanos: int64(i%seriesPerRowGroup+rowGroupNum*seriesPerRowGroup) * 1000,
		}

	}

	return createFileWith[T](t, rows, rowGroups)

}

func createFileWith[T any](t testing.TB, rows []T, rowGroups int) *parquet.File {
	f, err := os.CreateTemp(t.TempDir(), "data.parquet")
	require.NoError(t, err)
	t.Logf("Created temp file %s", f.Name())

	perRG := len(rows) / rowGroups

	w := parquet.NewGenericWriter[T](f)
	for i := 0; i < (rowGroups - 1); i++ {
		_, err = w.Write(rows[0:perRG])
		require.NoError(t, err)
		require.NoError(t, w.Flush())
		rows = rows[perRG:]
	}

	_, err = w.Write(rows)
	require.NoError(t, err)
	require.NoError(t, w.Flush())

	require.NoError(t, w.Close())

	stat, err := f.Stat()
	require.NoError(t, err)

	pf, err := parquet.OpenFile(f, stat.Size())
	require.NoError(t, err)

	return pf
}

func TestBinaryJoinIterator(t *testing.T) {
	rowCount := 1600
	pf := createProfileLikeFile(t, rowCount)

	for _, tc := range []struct {
		name                string
		seriesPredicate     Predicate
		seriesPageReads     int
		timePredicate       Predicate
		timePageReads       int
		expectedResultCount int
	}{
		{
			name:                "no predicate",
			expectedResultCount: rowCount, // expect everything
			seriesPageReads:     10,
			timePageReads:       10,
		},
		{
			name:                "one series ID",
			expectedResultCount: rowCount / 8, // expect an eight of the rows
			seriesPredicate:     NewMapPredicate(map[int64]struct{}{0: {}}),
			seriesPageReads:     10,
			timePageReads:       10,
		},
		{
			name:                "two series IDs",
			expectedResultCount: rowCount / 8 * 2, // expect two eights of the rows
			seriesPredicate:     NewMapPredicate(map[int64]struct{}{0: {}, 1: {}}),
			seriesPageReads:     10,
			timePageReads:       10,
		},
		{
			name:                "missing series",
			expectedResultCount: 0,
			seriesPredicate:     NewMapPredicate(map[int64]struct{}{10: {}}),
		},
		{
			name:                "first two time stamps each",
			expectedResultCount: 2 * 8, // expect two profiles for each series
			timePredicate:       NewIntBetweenPredicate(0, 1000),
			seriesPageReads:     1,
			timePageReads:       1,
		},
		{
			name:                "time before results",
			expectedResultCount: 0,
			timePredicate:       NewIntBetweenPredicate(-10, -1),
			seriesPageReads:     1,
			timePageReads:       0,
		},
		{
			name:                "time after results",
			expectedResultCount: 0,
			timePredicate:       NewIntBetweenPredicate(200000, 20001000),
			seriesPredicate:     NewMapPredicate(map[int64]struct{}{0: {}, 1: {}}),
			seriesPageReads:     1,
			timePageReads:       0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			reg := prometheus.NewRegistry()
			metrics := NewMetrics(reg)
			metrics.pageReadsTotal.WithLabelValues("ts", "SeriesId").Add(0)
			metrics.pageReadsTotal.WithLabelValues("ts", "TimeNanos").Add(0)
			ctx = AddMetricsToContext(ctx, metrics)

			seriesIt := NewSyncIterator(ctx, pf.RowGroups(), 0, "SeriesId", 1000, tc.seriesPredicate, "SeriesId")
			timeIt := NewSyncIterator(ctx, pf.RowGroups(), 1, "TimeNanos", 1000, tc.timePredicate, "TimeNanos")

			it := NewBinaryJoinIterator(
				0,
				seriesIt,
				timeIt,
			)

			results := 0
			for it.Next() {
				results++
			}
			require.NoError(t, it.Err())

			require.NoError(t, it.Close())

			require.Equal(t, tc.expectedResultCount, results)

			require.NoError(t, testutil.GatherAndCompare(reg, bytes.NewReader([]byte(fmt.Sprintf(
				`
        # HELP pyroscopedb_page_reads_total Total number of pages read while querying
        # TYPE pyroscopedb_page_reads_total counter
        pyroscopedb_page_reads_total{column="SeriesId",table="ts"} %d
        pyroscopedb_page_reads_total{column="TimeNanos",table="ts"} %d
        `, tc.seriesPageReads, tc.timePageReads))), "pyroscopedb_page_reads_total"))

		})
	}
}
