package parquetquery

import (
	"context"
	"errors"
	"testing"

	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"
)

type testData struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

func newTestBuffer[A any](rows []A) parquet.RowGroup {
	buffer := parquet.NewBuffer()
	for i := range rows {
		err := buffer.Write(rows[i])
		if err != nil {
			panic(err.Error())
		}
	}
	return buffer
}

type errRowGroup struct {
	parquet.RowGroup
}

func (e *errRowGroup) ColumnChunks() []parquet.ColumnChunk {
	chunks := e.RowGroup.ColumnChunks()
	for pos := range chunks {
		chunks[pos] = &errColumnChunk{chunks[pos]}
	}
	return chunks
}

type errColumnChunk struct {
	parquet.ColumnChunk
}

func (e *errColumnChunk) Pages() parquet.Pages {
	return &errPages{e.ColumnChunk.Pages()}
}

type errPages struct {
	parquet.Pages
}

func (e *errPages) ReadPage() (parquet.Page, error) {
	p, err := e.Pages.ReadPage()
	return &errPage{p}, err
}

type errPage struct {
	parquet.Page
}

func (e *errPage) Values() parquet.ValueReader {
	return &errValueReader{e.Page.Values()}
}

type errValueReader struct {
	parquet.ValueReader
}

func (e *errValueReader) ReadValues(vals []parquet.Value) (int, error) {
	_, _ = e.ValueReader.ReadValues(vals)
	return 0, errors.New("read error")
}

func withReadValueError(rg []parquet.RowGroup) []parquet.RowGroup {
	for pos := range rg {
		rg[pos] = &errRowGroup{rg[pos]}
	}
	return rg
}

func newTestSet() []parquet.RowGroup {
	return []parquet.RowGroup{
		newTestBuffer(
			[]testData{
				{1, "one"},
				{2, "two"},
			}),
		newTestBuffer(
			[]testData{
				{3, "three"},
				{5, "five"},
			}),
	}
}

func TestColumnIterator(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    []parquet.Value
		rowGroups []parquet.RowGroup
		err       error
	}{
		{
			name:      "read-int-column",
			rowGroups: newTestSet(),
			result: []parquet.Value{
				parquet.ValueOf(1),
				parquet.ValueOf(2),
				parquet.ValueOf(3),
				parquet.ValueOf(5),
			},
		},
		{
			name:      "err-read-values",
			rowGroups: withReadValueError(newTestSet()),
			err:       errors.New("read error"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				buffer [][]parquet.Value

				ctx = context.Background()
				i   = NewColumnIterator(ctx, tc.rowGroups, 0, "id", 10, nil, "id")
			)
			for i.Next() {
				require.Nil(t, i.Err())
				buffer = i.At().Columns(buffer, "id")
			}

			require.Equal(t, tc.err, i.Err())

		})
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
		require.Equal(t, tc.expected, CompareRowNumbers(5, tc.a, tc.b))
	}
}
