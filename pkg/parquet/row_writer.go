package parquet

import (
	"io"
)

// ColumnWriter is a minimal interface for writing column chunks
type ColumnWriter interface {
	WriteBatch(values interface{}, defLevels []int16, repLevels []int16) (int64, error)
}

type RowWriterFlusher interface {
	NextColumn() (ColumnWriter, error)
	NumRows() (int64, error)
	NumColumns() int
	Flush() error
}

// ColumnChunkReader is an interface that defines the methods needed to read column chunks
type ColumnChunkReader interface {
	// ReadBatch reads a batch of values and their definition/repetition levels.
	// It returns the total values read, number of rows read, and any error.
	ReadBatch(batchSize int64, values interface{}, defLevels, repLevels []int16) (valuesRead int64, rowsRead int, err error)
}

// RowGroupReader is an interface that defines the methods needed to read a parquet row group
type RowGroupReader interface {
	// NumRows returns the number of rows in the row group
	NumRows() int64
	// NumColumns returns the number of columns in the row group
	NumColumns() int
	// Column returns a column chunk reader for the given column index
	Column(i int) (ColumnChunkReader, error)
}

// CopyAsRowGroups copies row groups to dst from src and flush a rowgroup per rowGroupNumCount read.
// It returns the total number of rows copied and the number of row groups written.
// Flush is called to create a new row group.
func CopyAsRowGroups(dst RowWriterFlusher, src RowGroupReader, rowGroupNumCount int) (total int64, rowGroupCount int64, err error) {
	if rowGroupNumCount <= 0 {
		panic("rowGroupNumCount must be positive")
	}

	numRows := src.NumRows()
	if numRows == 0 {
		return 0, 0, nil
	}

	// Read column by column
	for colIdx := 0; colIdx < src.NumColumns(); colIdx++ {
		reader, err := src.Column(colIdx)
		if err != nil {
			return 0, 0, err
		}

		// Get the specific column reader type
		var values interface{}
		var read int
		var defLevels []int16
		var repLevels []int16

		// Pre-allocate slices for levels
		defLevels = make([]int16, rowGroupNumCount)
		repLevels = make([]int16, rowGroupNumCount)

		total, read, err = reader.ReadBatch(int64(rowGroupNumCount), values, defLevels, repLevels)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}
		if read == 0 {
			break
		}

		// Get the column writer from the row group writer
		writer, err := dst.NextColumn()
		if err != nil {
			return 0, 0, err
		}

		// Write the column chunk using the appropriate writer type
		written, err := writer.WriteBatch(values, defLevels, repLevels)
		if err != nil {
			return 0, 0, err
		}
		total += written

		// Check if we need to flush
		currentRows, err := dst.NumRows()
		if err != nil {
			return 0, 0, err
		}
		if int(currentRows) >= rowGroupNumCount {
			if err := dst.Flush(); err != nil {
				return 0, 0, err
			}
			rowGroupCount++
		}
	}

	// Flush any remaining data
	currentRows, err := dst.NumRows()
	if err != nil {
		return 0, 0, err
	}
	if currentRows > 0 {
		if err := dst.Flush(); err != nil {
			return 0, 0, err
		}
		rowGroupCount++
	}

	return total, rowGroupCount, nil
}
