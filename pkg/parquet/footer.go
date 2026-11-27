package parquet

import (
	"bytes"
	"fmt"

	parquetgo "github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/encoding/thrift"
)

// FooterSize returns the complete trailing region containing column indexes,
// offset indexes, file metadata, and the eight-byte trailer of an unencrypted file.
// The file view must belong to a closed writer or an opened file.
func FooterSize(f parquetgo.FileView) (uint64, error) {
	var metadata bytes.Buffer
	protocol := new(thrift.CompactProtocol)
	if err := thrift.NewEncoder(protocol.NewWriter(&metadata)).Encode(f.Metadata()); err != nil {
		return 0, fmt.Errorf("encoding parquet metadata: %w", err)
	}
	start := f.Size() - int64(metadata.Len()) - 8
	for _, rg := range f.Metadata().RowGroups {
		for _, column := range rg.Columns {
			if column.ColumnIndexOffset > 0 && column.ColumnIndexOffset < start {
				start = column.ColumnIndexOffset
			}
			if column.OffsetIndexOffset > 0 && column.OffsetIndexOffset < start {
				start = column.OffsetIndexOffset
			}
		}
	}
	if start < 4 || start > f.Size()-8 {
		return 0, fmt.Errorf("invalid parquet footer offset %d for size %d", start, f.Size())
	}
	return uint64(f.Size() - start), nil
}
