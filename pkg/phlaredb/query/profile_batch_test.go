package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileBatchScanner(t *testing.T) {
	file, rows := createProfileLikeFile(t, 1000)
	series, _ := GetColumnIndexByPath(file.Root(), "SeriesID")
	timestamp, _ := GetColumnIndexByPath(file.Root(), "TimeNanos")
	for _, size := range []int{1, 7, 256, 4096} {
		scanner, err := NewProfileBatchScanner(context.Background(), file.RowGroups(), series, timestamp, size, map[uint32]struct{}{1: {}, 3: {}}, 3000, 70000)
		require.NoError(t, err)
		var got []int64
		for scanner.Next() {
			batch := scanner.At()
			require.LessOrEqual(t, len(batch.RowNumbers), size)
			for i, row := range batch.RowNumbers {
				require.Equal(t, rows[row].SeriesID, batch.SeriesIndexes[i])
				require.Equal(t, rows[row].TimeNanos, batch.TimeNanos[i])
			}
			got = append(got, batch.RowNumbers...)
		}
		require.NoError(t, scanner.Err())
		require.NoError(t, scanner.Close())
		legacy := NewBinaryJoinIterator(0,
			NewSyncIterator(context.Background(), file.RowGroups(), series, "SeriesID", 1000, NewMapPredicate(map[int64]struct{}{1: {}, 3: {}}), ""),
			NewSyncIterator(context.Background(), file.RowGroups(), timestamp, "TimeNanos", 1000, NewIntBetweenPredicate(3000, 70000), ""))
		var want []int64
		for legacy.Next() {
			want = append(want, legacy.At().RowNumber[0])
		}
		require.NoError(t, legacy.Err())
		require.NoError(t, legacy.Close())
		require.Equal(t, want, got)
	}
}

func TestProfileBatchScannerCancellationAndEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	scanner, err := NewProfileBatchScanner(ctx, nil, 0, 1, 16, nil, 0, 1)
	require.NoError(t, err)
	cancel()
	require.False(t, scanner.Next())
	require.ErrorIs(t, scanner.Err(), context.Canceled)
	require.NoError(t, scanner.Close())
	require.NoError(t, scanner.Close())
	scanner, err = NewProfileBatchScanner(context.Background(), nil, 0, 1, 16, nil, 0, 1)
	require.NoError(t, err)
	require.False(t, scanner.Next())
	require.NoError(t, scanner.Err())
	require.NoError(t, scanner.Close())
	_, err = NewProfileBatchScanner(context.Background(), nil, 0, 1, 0, nil, 0, 1)
	require.Error(t, err)
}

func BenchmarkProfileBatchScanner(b *testing.B) {
	file, _ := createProfileLikeFile(b, 100000)
	series, _ := GetColumnIndexByPath(file.Root(), "SeriesID")
	timestamp, _ := GetColumnIndexByPath(file.Root(), "TimeNanos")
	for _, dense := range []bool{false, true} {
		name := "sparse"
		matches := map[uint32]struct{}{1: {}}
		if dense {
			name = "dense"
			for i := uint32(0); i < 8; i++ {
				matches[i] = struct{}{}
			}
		}
		b.Run(name, func(b *testing.B) {
			b.Run("batch", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					scanner, err := NewProfileBatchScanner(context.Background(), file.RowGroups(), series, timestamp, 1024, matches, 0, 1<<62)
					require.NoError(b, err)
					for scanner.Next() {
					}
					require.NoError(b, scanner.Err())
					require.NoError(b, scanner.Close())
				}
			})
			b.Run("scalar", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					scanner := NewBinaryJoinIterator(0,
						NewSyncIterator(context.Background(), file.RowGroups(), series, "SeriesID", 1000, NewMapPredicate(matches), "SeriesID"),
						NewSyncIterator(context.Background(), file.RowGroups(), timestamp, "TimeNanos", 1000, NewIntBetweenPredicate(0, 1<<62), "TimeNanos"))
					for scanner.Next() {
					}
					require.NoError(b, scanner.Err())
					require.NoError(b, scanner.Close())
				}
			})
		})
	}
}
