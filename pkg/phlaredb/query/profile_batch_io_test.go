package query

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
)

type batchCountingReader struct {
	*bytes.Reader
	calls atomic.Int64
	bytes atomic.Int64
}

func (r *batchCountingReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.Reader.ReadAt(p, off)
	r.calls.Add(1)
	r.bytes.Add(int64(n))
	return n, err
}

// ReaderAt counters measure reads below the Parquet reader, not remote object
// store requests. Run with a real object-store reader to evaluate remote costs.
func BenchmarkProfileBatchScannerIO(b *testing.B) {
	rows := make([]batchTestProfile, 100000)
	for i := range rows {
		rows[i] = batchTestProfile{SeriesIndex: uint32(i / 1000), TimeNanos: int64(i % 1000), StacktracePartition: uint64(i % 7)}
	}
	var data bytes.Buffer
	w := parquet.NewGenericWriter[batchTestProfile](&data, parquet.PageBufferSize(4096), parquet.MaxRowsPerRowGroup(10000))
	_, err := w.Write(rows)
	require.NoError(b, err)
	require.NoError(b, w.Close())
	for _, tc := range []struct {
		name       string
		matches    map[uint32]struct{}
		start, end int64
	}{
		{"sparse", map[uint32]struct{}{1: {}, 51: {}, 99: {}}, 0, 999},
		{"narrow", map[uint32]struct{}{1: {}, 51: {}, 99: {}}, 499, 501},
		{"empty", map[uint32]struct{}{1: {}}, 1001, 1002},
	} {
		for _, size := range []int{0, 256, 1024, 4096} {
			name := fmt.Sprintf("batch-%d", size)
			if size == 0 {
				name = "scalar"
			}
			b.Run(tc.name+"/"+name, func(b *testing.B) {
				reader := &batchCountingReader{Reader: bytes.NewReader(data.Bytes())}
				f, err := parquet.OpenFile(reader, int64(data.Len()))
				require.NoError(b, err)
				si, _ := GetColumnIndexByPath(f.Root(), "SeriesIndex")
				ti, _ := GetColumnIndexByPath(f.Root(), "TimeNanos")
				pi, _ := GetColumnIndexByPath(f.Root(), "StacktracePartition")
				expected := 0
				for _, row := range rows {
					if _, ok := tc.matches[row.SeriesIndex]; ok && row.TimeNanos >= tc.start && row.TimeNanos <= tc.end {
						expected++
					}
				}
				reader.calls.Store(0)
				reader.bytes.Store(0)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					count := 0
					if size == 0 {
						it := NewBinaryJoinIterator(0, NewBinaryJoinIterator(0,
							NewSyncIterator(context.Background(), f.RowGroups(), si, "SeriesIndex", 1000, NewMapPredicate(tc.matches), "SeriesIndex"),
							NewSyncIterator(context.Background(), f.RowGroups(), ti, "TimeNanos", 1000, NewIntBetweenPredicate(tc.start, tc.end), "TimeNanos")),
							NewSyncIterator(context.Background(), f.RowGroups(), pi, "StacktracePartition", 1000, nil, "StacktracePartition"))
						for it.Next() {
							count++
						}
						require.NoError(b, it.Err())
						require.NoError(b, it.Close())
					} else {
						it, err := NewProfileBatchScanner(context.Background(), f.RowGroups(), si, ti, size, tc.matches, tc.start, tc.end, pi)
						require.NoError(b, err)
						for it.Next() {
							count += len(it.At().RowNumbers)
						}
						require.NoError(b, it.Err())
						require.NoError(b, it.Close())
					}
					require.Equal(b, expected, count)
				}
				b.StopTimer()
				b.ReportMetric(float64(reader.calls.Load())/float64(b.N), "reads/op")
				b.ReportMetric(float64(reader.bytes.Load())/float64(b.N), "read-B/op")
			})
		}
	}
}

type batchBoundsIndex struct {
	parquet.ColumnIndex
	min, max parquet.Value
}

func (i batchBoundsIndex) NumPages() int              { return 1 }
func (i batchBoundsIndex) MinValue(int) parquet.Value { return i.min }
func (i batchBoundsIndex) MaxValue(int) parquet.Value { return i.max }

func TestProfileBatchUnusableStatistics(t *testing.T) {
	predicates := []Predicate{newProfileSeriesPredicate(map[uint32]struct{}{100: {}}), &profileTimePredicate{NewIntBetweenPredicate(100, 200)}}
	for _, p := range predicates {
		require.True(t, p.KeepColumnChunk(nil))
		require.True(t, p.KeepColumnChunk(batchBoundsIndex{}))
	}
	require.True(t, predicates[0].KeepColumnChunk(batchBoundsIndex{min: parquet.Int32Value(10), max: parquet.Int32Value(1)}))
	require.True(t, predicates[1].KeepColumnChunk(batchBoundsIndex{min: parquet.Int64Value(10), max: parquet.Int64Value(1)}))
}
