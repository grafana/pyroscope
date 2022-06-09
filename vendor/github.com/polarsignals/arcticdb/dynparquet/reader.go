package dynparquet

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/segmentio/parquet-go"
)

const (
	DynamicColumnsKey = "dynamic_columns"
)

var ErrNoDynamicColumns = errors.New("no dynamic columns metadata found, it must be present")

type SerializedBuffer struct {
	f       *parquet.File
	dynCols map[string][]string
}

func ReaderFromBytes(buf []byte) (*SerializedBuffer, error) {
	f, err := parquet.OpenFile(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, err
	}

	return NewSerializedBuffer(f)
}

func NewSerializedBuffer(f *parquet.File) (*SerializedBuffer, error) {
	dynColString, found := f.Lookup(DynamicColumnsKey)
	if !found {
		return nil, ErrNoDynamicColumns
	}

	dynCols, err := deserializeDynamicColumns(dynColString)
	if err != nil {
		return nil, fmt.Errorf("deserialize dynamic columns metadata %q: %w", dynColString, err)
	}

	return &SerializedBuffer{
		f:       f,
		dynCols: dynCols,
	}, nil
}

func (b *SerializedBuffer) Reader() *parquet.Reader {
	return parquet.NewReader(b.ParquetFile())
}

func (b *SerializedBuffer) ParquetFile() *parquet.File {
	return b.f
}

func (b *SerializedBuffer) NumRows() int64 {
	return b.Reader().NumRows()
}

func (b *SerializedBuffer) NumRowGroups() int {
	return len(b.f.RowGroups())
}

func (b *SerializedBuffer) DynamicRows() DynamicRowReader {
	rowGroups := b.f.RowGroups()
	drg := make([]DynamicRowGroup, len(rowGroups))
	for i, rowGroup := range rowGroups {
		drg[i] = b.newDynamicRowGroup(rowGroup)
	}
	return Concat(drg...).DynamicRows()
}

type serializedRowGroup struct {
	parquet.RowGroup
	dynCols map[string][]string
}

func (b *SerializedBuffer) DynamicRowGroup(i int) DynamicRowGroup {
	return b.newDynamicRowGroup(b.f.RowGroups()[i])
}

func (b *SerializedBuffer) newDynamicRowGroup(rowGroup parquet.RowGroup) DynamicRowGroup {
	return &serializedRowGroup{
		RowGroup: rowGroup,
		dynCols:  b.dynCols,
	}
}

func (g *serializedRowGroup) DynamicColumns() map[string][]string {
	return g.dynCols
}

func (g *serializedRowGroup) DynamicRows() DynamicRowReader {
	return newDynamicRowGroupReader(g)
}

func (b *SerializedBuffer) DynamicColumns() map[string][]string {
	return b.dynCols
}
