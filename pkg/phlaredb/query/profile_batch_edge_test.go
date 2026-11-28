package query

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
)

type batchTestProfile struct {
	SeriesIndex         uint32
	TimeNanos           int64
	StacktracePartition uint64
}

func batchTestFile(t testing.TB, rows []batchTestProfile) *parquet.File {
	t.Helper()
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[batchTestProfile](&buf, parquet.PageBufferSize(128), parquet.MaxRowsPerRowGroup(73))
	_, err := w.Write(rows)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	f, err := parquet.OpenFile(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return f
}

func TestProfileBatchScannerBoundaries(t *testing.T) {
	rows := make([]batchTestProfile, 997)
	for i := range rows {
		rows[i] = batchTestProfile{SeriesIndex: uint32(i / 29), TimeNanos: int64(i%37) - 18, StacktracePartition: uint64(i % 11)}
		if i%7 == 0 {
			rows[i].SeriesIndex = 1<<31 + 5
			rows[i].StacktracePartition = 1<<63 + 7
		}
	}
	f := batchTestFile(t, rows)
	si, _ := GetColumnIndexByPath(f.Root(), "SeriesIndex")
	ti, _ := GetColumnIndexByPath(f.Root(), "TimeNanos")
	pi, _ := GetColumnIndexByPath(f.Root(), "StacktracePartition")
	for _, size := range []int{1, 3, 64, 1024} {
		for _, matches := range []map[uint32]struct{}{nil, {2: {}}, {1<<31 + 5: {}, 2: {}, 30: {}}} {
			for _, bounds := range [][2]int64{{-18, 36}, {-18, -1}, {7, 7}, {14, 26}, {50, 60}, {10, 0}} {
				s, err := NewProfileBatchScanner(context.Background(), f.RowGroups(), si, ti, size, matches, bounds[0], bounds[1], pi)
				require.NoError(t, err)
				var got, want []int64
				for s.Next() {
					for _, cursor := range []*flatProfileCursor{s.series, s.times, s.partitions} {
						require.LessOrEqual(t, cap(cursor.rows), size)
						require.LessOrEqual(t, cap(cursor.values), size)
						require.LessOrEqual(t, cap(cursor.it.currBuf), size)
					}
					b := s.At()
					for j, row := range b.RowNumbers {
						got = append(got, row)
						require.Equal(t, rows[row].SeriesIndex, b.SeriesIndexes[j])
						require.Equal(t, rows[row].TimeNanos, b.TimeNanos[j])
						require.Equal(t, rows[row].StacktracePartition, b.Partitions[j])
					}
				}
				require.NoError(t, s.Err())
				require.NoError(t, s.Close())
				require.NoError(t, s.Close())
				require.False(t, s.Next())
				for i, row := range rows {
					if _, ok := matches[row.SeriesIndex]; ok && row.TimeNanos >= bounds[0] && row.TimeNanos <= bounds[1] {
						want = append(want, int64(i))
					}
				}
				require.Equal(t, want, got)
			}
		}
	}
}

func TestProfileBatchScannerUnsupportedSchema(t *testing.T) {
	type optional struct {
		SeriesIndex *uint32
		TimeNanos   int64
	}
	v := uint32(1)
	f := createFileWith(t, []optional{{&v, 1}}, 1)
	_, err := NewProfileBatchScanner(context.Background(), f.RowGroups(), 0, 1, 16, map[uint32]struct{}{1: {}}, 0, 2)
	require.ErrorIs(t, err, ErrUnsupportedProfileBatchSchema)

	signed := createFileWith(t, []struct {
		SeriesIndex int32
		TimeNanos   int64
	}{{1, 1}}, 1)
	_, err = NewProfileBatchScanner(context.Background(), signed.RowGroups(), 0, 1, 16, map[uint32]struct{}{1: {}}, 0, 2)
	require.ErrorIs(t, err, ErrUnsupportedProfileBatchSchema)
	repeated := createFileWith(t, []struct {
		SeriesIndex []uint32
		TimeNanos   int64
	}{{[]uint32{1}, 1}}, 1)
	_, err = NewProfileBatchScanner(context.Background(), repeated.RowGroups(), 0, 1, 16, map[uint32]struct{}{1: {}}, 0, 2)
	require.ErrorIs(t, err, ErrUnsupportedProfileBatchSchema)
	for _, columns := range [][2]int{{-1, 1}, {0, 100}, {0, 0}} {
		_, err = NewProfileBatchScanner(context.Background(), f.RowGroups(), columns[0], columns[1], 16, map[uint32]struct{}{1: {}}, 0, 2)
		require.ErrorIs(t, err, ErrUnsupportedProfileBatchSchema)
	}
}

type failingBatchGroup struct {
	parquet.RowGroup
	failure      error
	closeFailure bool
}

func (g failingBatchGroup) ColumnChunks() []parquet.ColumnChunk {
	chunks := append([]parquet.ColumnChunk(nil), g.RowGroup.ColumnChunks()...)
	for i, c := range chunks {
		chunks[i] = failingBatchChunk{ColumnChunk: c, failure: g.failure, closeFailure: g.closeFailure}
	}
	return chunks
}

type failingBatchChunk struct {
	parquet.ColumnChunk
	failure      error
	closeFailure bool
}

func (c failingBatchChunk) Pages() parquet.Pages {
	return &failingBatchPages{Pages: c.ColumnChunk.Pages(), failure: c.failure, closeFailure: c.closeFailure}
}

type failingBatchPages struct {
	parquet.Pages
	failure      error
	closeFailure bool
}

func (p *failingBatchPages) ReadPage() (parquet.Page, error) {
	if !p.closeFailure {
		return nil, p.failure
	}
	return p.Pages.ReadPage()
}
func (p *failingBatchPages) Close() error {
	err := p.Pages.Close()
	if p.closeFailure {
		return p.failure
	}
	return err
}

func TestProfileBatchScannerReadAndCloseErrors(t *testing.T) {
	f := batchTestFile(t, []batchTestProfile{{1, 1, 3}})
	si, _ := GetColumnIndexByPath(f.Root(), "SeriesIndex")
	ti, _ := GetColumnIndexByPath(f.Root(), "TimeNanos")
	failure := errors.New("injected reader failure")
	for _, closeFailure := range []bool{false, true} {
		groups := []parquet.RowGroup{failingBatchGroup{RowGroup: f.RowGroups()[0], failure: failure, closeFailure: closeFailure}}
		s, err := NewProfileBatchScanner(context.Background(), groups, si, ti, 16, map[uint32]struct{}{1: {}}, 0, 2)
		require.NoError(t, err)
		for s.Next() {
		}
		if closeFailure {
			require.ErrorIs(t, s.Close(), failure)
		} else {
			require.ErrorIs(t, s.Err(), failure)
			require.NoError(t, s.Close())
		}
	}
}

func TestProfileBatchScannerCancellationAfterBatch(t *testing.T) {
	rows := make([]batchTestProfile, 100)
	for i := range rows {
		rows[i] = batchTestProfile{1, int64(i), 3}
	}
	f := batchTestFile(t, rows)
	si, _ := GetColumnIndexByPath(f.Root(), "SeriesIndex")
	ti, _ := GetColumnIndexByPath(f.Root(), "TimeNanos")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := NewProfileBatchScanner(ctx, f.RowGroups(), si, ti, 3, map[uint32]struct{}{1: {}}, 0, 100)
	require.NoError(t, err)
	require.True(t, s.Next())
	cancel()
	require.False(t, s.Next())
	require.ErrorIs(t, s.Err(), context.Canceled)
	require.NoError(t, s.Close())
}
