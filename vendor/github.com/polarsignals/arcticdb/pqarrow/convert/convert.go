package convert

import (
	"errors"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/segmentio/parquet-go"

	"github.com/polarsignals/arcticdb/pqarrow/writer"
)

// ParquetNodeToType converts a parquet node to an arrow type.
func ParquetNodeToType(n parquet.Node) (arrow.DataType, error) {
	typ, _, err := ParquetNodeToTypeWithWriterFunc(n)
	if err != nil {
		return nil, err
	}
	return typ, nil
}

// ParquetNodeToTypeWithWriterFunc converts a parquet node to an arrow type and a function to
// create a value writer.
func ParquetNodeToTypeWithWriterFunc(n parquet.Node) (arrow.DataType, func(b array.Builder, numValues int) writer.ValueWriter, error) {
	t := n.Type()
	lt := t.LogicalType()

	if lt == nil {
		return nil, nil, errors.New("unsupported type")
	}

	switch {
	case lt.UTF8 != nil:
		return &arrow.BinaryType{}, writer.NewBinaryValueWriter, nil
	case lt.Integer != nil:
		switch lt.Integer.BitWidth {
		case 64:
			if lt.Integer.IsSigned {
				return &arrow.Int64Type{}, writer.NewInt64ValueWriter, nil
			}
			return &arrow.Uint64Type{}, writer.NewUint64ValueWriter, nil
		default:
			return nil, nil, errors.New("unsupported int bit width")
		}
	default:
		return nil, nil, errors.New("unsupported type")
	}
}
