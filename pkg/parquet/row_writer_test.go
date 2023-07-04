package parquet

import (
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

var _ RowGroupWriter = (*TestRowGroupWriter)(nil)

type TestRowGroupWriter struct {
	RowGroups       [][]parquet.Row
	currentRowGroup int
}

func (r *TestRowGroupWriter) WriteRows(rows []parquet.Row) (int, error) {
	if len(r.RowGroups) <= r.currentRowGroup {
		r.RowGroups = append(r.RowGroups, []parquet.Row{})
	}
	r.RowGroups[r.currentRowGroup] = append(r.RowGroups[r.currentRowGroup], rows...)
	return len(rows), nil
}

func (r *TestRowGroupWriter) Flush() error {
	r.currentRowGroup++
	return nil
}

func TestCopyAsRowGroups(t *testing.T) {
	for _, tc := range []struct {
		name             string
		rowGroupNumCount int
		reader           parquet.RowReader
		expected         [][]parquet.Row
	}{
		{
			"empty",
			1,
			EmptyRowReader,
			nil,
		},
		{
			"one row",
			1,
			NewBatchReader([][]parquet.Row{
				{{parquet.Int32Value(1)}},
			}),
			[][]parquet.Row{
				{{parquet.Int32Value(1)}},
			},
		},
		{
			"one row per group",
			1,
			NewBatchReader([][]parquet.Row{
				{{parquet.Int32Value(1)}},
				{{parquet.Int32Value(2)}, {parquet.Int32Value(3)}},
				{{parquet.Int32Value(4)}},
			}),
			[][]parquet.Row{
				{{parquet.Int32Value(1)}},
				{{parquet.Int32Value(2)}},
				{{parquet.Int32Value(3)}},
				{{parquet.Int32Value(4)}},
			},
		},
		{
			"two row per group",
			2,
			NewBatchReader([][]parquet.Row{
				{{parquet.Int32Value(1)}},
				{{parquet.Int32Value(2)}, {parquet.Int32Value(3)}},
				{{parquet.Int32Value(4)}},
			}),
			[][]parquet.Row{
				{{parquet.Int32Value(1)}, {parquet.Int32Value(2)}},
				{{parquet.Int32Value(3)}, {parquet.Int32Value(4)}},
			},
		},
		{
			"two row per group not full",
			2,
			NewBatchReader([][]parquet.Row{
				{{parquet.Int32Value(1)}},
				{{parquet.Int32Value(2)}, {parquet.Int32Value(3)}, {parquet.Int32Value(4)}, {parquet.Int32Value(5)}},
			}),
			[][]parquet.Row{
				{{parquet.Int32Value(1)}, {parquet.Int32Value(2)}},
				{{parquet.Int32Value(3)}, {parquet.Int32Value(4)}},
				{{parquet.Int32Value(5)}},
			},
		},
		{
			"more in the group than the reader can read",
			10000,
			NewBatchReader([][]parquet.Row{
				{{parquet.Int32Value(1)}},
				{{parquet.Int32Value(2)}, {parquet.Int32Value(3)}, {parquet.Int32Value(4)}, {parquet.Int32Value(5)}},
			}),
			[][]parquet.Row{
				{
					{parquet.Int32Value(1)},
					{parquet.Int32Value(2)},
					{parquet.Int32Value(3)},
					{parquet.Int32Value(4)},
					{parquet.Int32Value(5)},
				},
			},
		},
		{
			"more in the reader",
			10000,
			NewBatchReader([][]parquet.Row{
				generateRows(5000),
				generateRows(3000),
			}),
			[][]parquet.Row{
				append(generateRows(5000), generateRows(3000)...),
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			writer := &TestRowGroupWriter{}
			total, rowGroupCount, err := CopyAsRowGroups(writer, tc.reader, tc.rowGroupNumCount)
			require.NoError(t, err)
			require.Equal(t, uint64(countRows(tc.expected)), total)
			require.Equal(t, uint64(len(tc.expected)), rowGroupCount)
			require.Equal(t, tc.expected, writer.RowGroups)
		})
	}
}

func countRows(rows [][]parquet.Row) int {
	count := 0
	for _, r := range rows {
		count += len(r)
	}
	return count
}

func generateRows(count int) []parquet.Row {
	rows := make([]parquet.Row, count)
	for i := 0; i < count; i++ {
		rows[i] = parquet.Row{parquet.Int32Value(int32(i))}
	}
	return rows
}
