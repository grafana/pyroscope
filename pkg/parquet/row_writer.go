package parquet

import (
	"io"

	"github.com/segmentio/parquet-go"
)

type RowWriterFlusher interface {
	parquet.RowWriter
	Flush() error
}

// CopyAsRowGroups copies row groups to dst from src and flush a rowgroup per rowGroupNumCount read.
// It returns the total number of rows copied and the number of row groups written.
// Flush is called to create a new row group.
func CopyAsRowGroups(dst RowWriterFlusher, src parquet.RowReader, rowGroupNumCount int) (total uint64, rowGroupCount uint64, err error) {
	if rowGroupNumCount <= 0 {
		panic("rowGroupNumCount must be positive")
	}
	bufferSize := defaultRowBufferSize
	// We clamp the buffer to the rowGroupNumCount to avoid allocating a buffer that is too large.
	if rowGroupNumCount < bufferSize {
		bufferSize = rowGroupNumCount
	}
	var (
		buffer            = make([]parquet.Row, bufferSize)
		currentGroupCount int
	)

	for {
		n, err := src.ReadRows(buffer[:bufferSize])
		if err != nil && err != io.EOF {
			return 0, 0, err
		}
		if n == 0 {
			break
		}
		buffer := buffer[:n]
		if currentGroupCount+n >= rowGroupNumCount {
			batchSize := rowGroupNumCount - currentGroupCount
			written, err := dst.WriteRows(buffer[:batchSize])
			if err != nil {
				return 0, 0, err
			}
			buffer = buffer[batchSize:]
			total += uint64(written)
			if err := dst.Flush(); err != nil {
				return 0, 0, err
			}
			rowGroupCount++
			currentGroupCount = 0
		}
		if len(buffer) == 0 {
			continue
		}
		written, err := dst.WriteRows(buffer)
		if err != nil {
			return 0, 0, err
		}
		total += uint64(written)
		currentGroupCount += written
	}
	if currentGroupCount > 0 {
		if err := dst.Flush(); err != nil {
			return 0, 0, err
		}
		rowGroupCount++
	}
	return
}
