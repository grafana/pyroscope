package query

import (
	"testing"

	"github.com/grafana/fire/pkg/iter"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

type RepeatedTestRow struct {
	Id   int
	List []int64
}

type testRowGetter struct {
	rowNumber int64
}

func (t testRowGetter) RowNumber() int64 {
	return t.rowNumber
}

func Test_RepeatedIterator(t *testing.T) {
	defaultReadSize := 100
	for _, tc := range []struct {
		name     string
		rows     []testRowGetter
		rgs      [][]RepeatedTestRow
		expected []RepeatedRow[testRowGetter]
		readSize int
	}{
		{
			name: "single row group",
			rows: []testRowGetter{
				{0},
				{1},
				{2},
			},
			rgs: [][]RepeatedTestRow{
				{
					{1, []int64{1, 2, 3}},
					{2, []int64{4, 5, 6}},
					{3, []int64{7, 8, 9}},
				},
			},
			expected: []RepeatedRow[testRowGetter]{
				{testRowGetter{0}, []parquet.Value{parquet.ValueOf(1), parquet.ValueOf(2), parquet.ValueOf(3)}},
				{testRowGetter{1}, []parquet.Value{parquet.ValueOf(4), parquet.ValueOf(5), parquet.ValueOf(6)}},
				{testRowGetter{2}, []parquet.Value{parquet.ValueOf(7), parquet.ValueOf(8), parquet.ValueOf(9)}},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var groups []parquet.RowGroup
			for _, rg := range tc.rgs {
				buffer := parquet.NewBuffer()
				for _, row := range rg {
					require.NoError(t, buffer.Write(row))
				}
				groups = append(groups, buffer)
			}
			if tc.readSize == 0 {
				tc.readSize = defaultReadSize
			}
			actual, err := iter.Slice(NewRepeatedPageIterator(
				iter.NewSliceIterator(tc.rows), groups, 1, tc.readSize))
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
