package parquet

import (
	"fmt"
	"io"
	"testing"

	pq "github.com/apache/arrow/go/v15/parquet"
	"github.com/apache/arrow/go/v15/parquet/schema"
	"github.com/stretchr/testify/require"
)

type TestRowWriterFlusher struct {
	values        []interface{}
	currentRows   int
	rowGroupSizes []int
	rowGroupCount int
	currentCol    int
}

type TestColumnWriter struct {
	writer *TestRowWriterFlusher
}

func (w TestColumnWriter) WriteBatch(values interface{}, defLevels []int16, repLevels []int16) (int64, error) {
	vals, ok := values.([]int32)
	if !ok {
		return 0, fmt.Errorf("expected []int32, got %T", values)
	}
	for _, v := range vals {
		w.writer.values = append(w.writer.values, v)
	}
	w.writer.currentRows += len(vals)
	return int64(len(vals)), nil
}

func (r *TestRowWriterFlusher) NextColumn() (ColumnWriter, error) {
	if r.currentCol >= r.NumColumns() {
		return nil, fmt.Errorf("no more columns")
	}
	writer := TestColumnWriter{writer: r}
	r.currentCol++
	return writer, nil
}

func (r *TestRowWriterFlusher) NumRows() (int64, error) {
	return int64(r.currentRows), nil
}

func (r *TestRowWriterFlusher) NumColumns() int { 
	return 1 
}

func (r *TestRowWriterFlusher) Flush() error {
	r.rowGroupSizes = append(r.rowGroupSizes, r.currentRows)
	r.currentRows = 0
	r.rowGroupCount++
	r.currentCol = 0
	return nil
}

type TestRowGroupReader struct {
	values    []int32
	position  int
	numRows   int64
}

func NewTestRowGroupReader(values []int32) *TestRowGroupReader {
	return &TestRowGroupReader{
		values:   values,
		numRows:  int64(len(values)),
	}
}

// Implementing required RowGroupReader interface methods
func (r *TestRowGroupReader) NumRows() int64 { return r.numRows }
func (r *TestRowGroupReader) NumColumns() int { return 1 }
func (r *TestRowGroupReader) Column(i int) (ColumnChunkReader, error) {
	return TestColumnChunkReader{reader: r}, nil
}

type TestColumnChunkReader struct {
	reader *TestRowGroupReader
}

func (r TestColumnChunkReader) ReadBatch(max int64, values interface{}, defLevels []int16, repLevels []int16) (int64, int, error) {
	if r.reader.position >= len(r.reader.values) {
		return 0, 0, io.EOF
	}

	remaining := len(r.reader.values) - r.reader.position
	batchSize := int(max)
	if remaining < batchSize {
		batchSize = remaining
	}

	// Type assert and handle the values slice
	valuesSlice, ok := values.([]int32)
	if !ok {
		// If values is nil or wrong type, create a new slice
		valuesSlice = make([]int32, batchSize)
	} else if len(valuesSlice) < batchSize {
		// Ensure the slice is large enough
		valuesSlice = make([]int32, batchSize)
	}

	copy(valuesSlice, r.reader.values[r.reader.position:r.reader.position+batchSize])
	r.reader.position += batchSize

	// If values was passed as a slice, update it with the new data
	if v, ok := values.(*[]int32); ok {
		*v = valuesSlice
	}

	return int64(batchSize), batchSize, nil
}

func TestCopyAsRowGroups(t *testing.T) {
	tests := []struct {
		name             string
		values          []int32
		rowGroupNumCount int
		expectedSizes    []int
		expectedTotal    int64
		expectedGroups   int64
	}{
		{
			name:             "empty",
			values:          []int32{},
			rowGroupNumCount: 1,
			expectedSizes:    nil,
			expectedTotal:    0,
			expectedGroups:   0,
		},
		{
			name:             "single value",
			values:          []int32{1},
			rowGroupNumCount: 1,
			expectedSizes:    []int{1},
			expectedTotal:    1,
			expectedGroups:   1,
		},
		{
			name:             "multiple values single group",
			values:          []int32{1, 2, 3, 4},
			rowGroupNumCount: 5,
			expectedSizes:    []int{4},
			expectedTotal:    4,
			expectedGroups:   1,
		},
		{
			name:             "multiple values multiple groups",
			values:          []int32{1, 2, 3, 4, 5},
			rowGroupNumCount: 2,
			expectedSizes:    []int{2, 2, 1},
			expectedTotal:    5,
			expectedGroups:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &TestRowWriterFlusher{}
			reader := NewTestRowGroupReader(tt.values)
			
			total, groups, err := CopyAsRowGroups(writer, reader, tt.rowGroupNumCount)
			
			require.NoError(t, err)
			require.Equal(t, tt.expectedTotal, total)
			require.Equal(t, tt.expectedGroups, groups)
			require.Equal(t, tt.expectedSizes, writer.rowGroupSizes)
			
			// Convert interface{} values back to int32 for comparison
			actualValues := make([]int32, len(writer.values))
			for i, v := range writer.values {
				actualValues[i] = v.(int32)
			}
			require.Equal(t, tt.values, actualValues)
		})
	}
}

// Helper function to create a simple schema for testing
func createTestSchema() *schema.Schema {
	intNode := schema.MustPrimitive(schema.NewPrimitiveNode("int32_field", pq.Repetitions.Required, pq.Types.Int32, -1, -1))
	fields := schema.FieldList{intNode}
	return schema.NewSchema(schema.MustGroup(schema.NewGroupNode("schema", pq.Repetitions.Required, fields, -1)))
}

// Helper function to create a parquet value
func Int32Value(v int32) interface{} {
	return v
}

func countRows(rows [][]interface{}) int {
	count := 0
	for _, r := range rows {
		count += len(r)
	}
	return count
}

func generateRows(count int) []interface{} {
	rows := make([]interface{}, count)
	for i := 0; i < count; i++ {
		rows[i] = Int32Value(int32(i))
	}
	return rows
}
