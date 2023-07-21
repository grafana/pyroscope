package parquet

import (
	"fmt"
	"io"
	"math"
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

var _ parquet.RowReader = (*BatchReader)(nil)

type BatchReader struct {
	batches [][]parquet.Row
}

func NewBatchReader(batches [][]parquet.Row) *BatchReader {
	return &BatchReader{batches: batches}
}

func (br *BatchReader) ReadRows(rows []parquet.Row) (int, error) {
	if len(br.batches) == 0 {
		return 0, io.EOF
	}
	n := copy(rows, br.batches[0])
	if n < len(br.batches[0]) {
		br.batches[0] = br.batches[0][n:]
		return n, nil
	}
	br.batches = br.batches[1:]
	return n, nil
}

func TestBufferedRowReaderIterator(t *testing.T) {
	testBatchSize := func(n int) func(t *testing.T) {
		return func(t *testing.T) {
			reader := NewBufferedRowReaderIterator(
				NewBatchReader(
					[][]parquet.Row{
						{{parquet.Int32Value(1)}},
						{{parquet.Int32Value(2)}, {parquet.Int32Value(3)}},
						{{parquet.Int32Value(4)}},
					}),
				n)
			require.True(t, reader.Next())
			require.Equal(t, parquet.Int32Value(1), reader.At()[0])
			require.True(t, reader.Next())
			require.Equal(t, parquet.Int32Value(2), reader.At()[0])
			require.True(t, reader.Next())
			require.Equal(t, parquet.Int32Value(3), reader.At()[0])
			require.True(t, reader.Next())
			require.Equal(t, parquet.Int32Value(4), reader.At()[0])
			require.False(t, reader.Next())
		}
	}
	t.Run("batch of 1", testBatchSize(1))
	t.Run("bigger batch", testBatchSize(100))
	t.Run("equal batch", testBatchSize(2))
}

func TestNewMergeRowReader(t *testing.T) {
	for _, batchSize := range []int{1, 2, 3, 4, 5, 6} {
		bufferSize := batchSize
		t.Run(fmt.Sprintf("%d", bufferSize), func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				readers  []parquet.RowReader
				expected []parquet.Row
			}{
				{
					"merge 1 readers",
					[]parquet.RowReader{
						NewBatchReader([][]parquet.Row{
							{{parquet.Int32Value(1)}},
							{{parquet.Int32Value(3)}},
							{{parquet.Int32Value(5)}},
						}),
					},
					[]parquet.Row{
						{parquet.Int32Value(1)},
						{parquet.Int32Value(3)},
						{parquet.Int32Value(5)},
					},
				},
				{
					"merge 2 readers",
					[]parquet.RowReader{
						NewBatchReader([][]parquet.Row{
							{{parquet.Int32Value(1)}},
							{{parquet.Int32Value(3)}},
							{{parquet.Int32Value(5)}},
						}),
						NewBatchReader([][]parquet.Row{
							{{parquet.Int32Value(2)}},
							{{parquet.Int32Value(4)}},
							{{parquet.Int32Value(6)}},
						}),
					},
					[]parquet.Row{
						{parquet.Int32Value(1)},
						{parquet.Int32Value(2)},
						{parquet.Int32Value(3)},
						{parquet.Int32Value(4)},
						{parquet.Int32Value(5)},
						{parquet.Int32Value(6)},
					},
				},
				{
					"merge 3 readers 1 value",
					[]parquet.RowReader{
						NewBatchReader([][]parquet.Row{
							{{parquet.Int32Value(1)}},
						}),
						NewBatchReader([][]parquet.Row{
							{{parquet.Int32Value(2)}},
						}),
						NewBatchReader([][]parquet.Row{
							{{parquet.Int32Value(3)}},
						}),
					},
					[]parquet.Row{
						{parquet.Int32Value(1)},
						{parquet.Int32Value(2)},
						{parquet.Int32Value(3)},
					},
				},
			} {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					reader := NewMergeRowReader(tc.readers, parquet.Row{parquet.Int32Value(math.MaxInt32)}, func(r1, r2 parquet.Row) bool {
						return r1[0].Int32() < r2[0].Int32()
					})

					actual, err := ReadAllWithBufferSize(reader, bufferSize)
					require.NoError(t, err)
					require.Equal(t, tc.expected, actual)
				})
			}
		})
	}
}

func TestIteratorRowReader(t *testing.T) {
	it := NewIteratorRowReader(
		NewBufferedRowReaderIterator(NewBatchReader([][]parquet.Row{
			{{parquet.Int32Value(1)}, {parquet.Int32Value(2)}, {parquet.Int32Value(3)}},
			{{parquet.Int32Value(4)}, {parquet.Int32Value(5)}, {parquet.Int32Value(6)}},
			{{parquet.Int32Value(7)}, {parquet.Int32Value(8)}, {parquet.Int32Value(9)}},
		}), 4),
	)
	actual, err := ReadAllWithBufferSize(it, 3)
	require.NoError(t, err)
	require.Equal(t, []parquet.Row{
		{parquet.Int32Value(1)},
		{parquet.Int32Value(2)},
		{parquet.Int32Value(3)},
		{parquet.Int32Value(4)},
		{parquet.Int32Value(5)},
		{parquet.Int32Value(6)},
		{parquet.Int32Value(7)},
		{parquet.Int32Value(8)},
		{parquet.Int32Value(9)},
	}, actual)
}

type SomeRow struct {
	Col1 int
}

func BenchmarkBufferedRowReader(b *testing.B) {
	buff := parquet.NewGenericBuffer[SomeRow]()
	for i := 0; i < 1000000; i++ {
		_, err := buff.Write([]SomeRow{{Col1: (i)}})
		if err != nil {
			b.Fatal(err)
		}
	}
	reader := NewBufferedRowReaderIterator(buff.Rows(), 100)
	defer reader.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for reader.Next() {
			_ = reader.At()
		}
	}
}
