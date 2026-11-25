package querybackend

import (
	"testing"

	"github.com/grafana/pyroscope/v2/pkg/iter"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"
	"github.com/stretchr/testify/require"
)

func TestSpanProfileIterator(t *testing.T) {
	entries := []ProfileEntry{{RowNum: 0}, {RowNum: 2}, {RowNum: 3}, {RowNum: 5}, {RowNum: 7}, {RowNum: 9}}
	filtered := &spanProfileIterator{
		Iterator: iter.NewSliceIterator(entries),
		ranges:   [][2]int64{{2, 4}, {7, 9}},
	}
	var rows []int64
	for filtered.Next() {
		rows = append(rows, filtered.At().RowNum)
	}
	require.NoError(t, filtered.Err())
	require.Equal(t, []int64{2, 3, 7}, rows)
	require.NoError(t, filtered.Close())
}

func TestColumnMayHaveSpans(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index *format.ColumnIndex
		want  bool
	}{
		{"missing", nil, true},
		{"empty", &format.ColumnIndex{}, true},
		{"null", &format.ColumnIndex{NullPages: []bool{true}, MinValues: [][]byte{nil}, MaxValues: [][]byte{nil}}, false},
		{"zero", &format.ColumnIndex{MinValues: [][]byte{make([]byte, 8)}, MaxValues: [][]byte{make([]byte, 8)}}, false},
		{"nonzero", &format.ColumnIndex{MinValues: [][]byte{make([]byte, 8)}, MaxValues: [][]byte{{0, 0, 0, 0, 0, 0, 0, 128}}}, true},
		{"mixed", &format.ColumnIndex{NullPages: []bool{true, false}, MinValues: [][]byte{nil, make([]byte, 8)}, MaxValues: [][]byte{nil, {1, 0, 0, 0, 0, 0, 0, 0}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ci parquet.ColumnIndex
			if tc.index != nil {
				ci = parquet.NewColumnIndex(parquet.Int64, tc.index)
			}
			require.Equal(t, tc.want, columnMayHaveSpans(ci))
		})
	}
}
