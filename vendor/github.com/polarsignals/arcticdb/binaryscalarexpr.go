package arcticdb

import (
	"errors"

	"github.com/segmentio/parquet-go"

	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/polarsignals/arcticdb/query/logicalplan"
)

var ErrUnexpectedNumberOfFields = errors.New("unexpected number of fields")

type ColumnRef struct {
	ColumnName string
}

func (c *ColumnRef) Column(rg dynparquet.DynamicRowGroup) (parquet.ColumnChunk, bool, error) {
	columnIndex := findColumnIndex(rg.Schema(), c.ColumnName)
	var columnChunk parquet.ColumnChunk
	// columnChunk can be nil if the column is not present in the row group.
	if columnIndex != -1 {
		columnChunk = rg.ColumnChunks()[columnIndex]
	}

	return columnChunk, columnIndex != -1, nil
}

func findColumnIndex(s *parquet.Schema, columnName string) int {
	for i, field := range s.Fields() {
		if field.Name() == columnName {
			return i
		}
	}
	return -1
}

type BinaryScalarExpr struct {
	Left  *ColumnRef
	Op    logicalplan.Operator
	Right parquet.Value
}

func (e BinaryScalarExpr) Eval(rg dynparquet.DynamicRowGroup) (bool, error) {
	leftData, exists, err := e.Left.Column(rg)
	if err != nil {
		return false, err
	}

	// TODO: This needs a bunch of test cases to validate edge cases like non
	// existant columns or null values. I'm pretty sure this is completely
	// wrong and needs per operation, per type specific behavior.
	if !exists {
		return false, nil
	}

	return BinaryScalarOperation(leftData, e.Right, e.Op)
}

var ErrUnsupportedBinaryOperation = errors.New("unsupported binary operation")

func BinaryScalarOperation(left parquet.ColumnChunk, right parquet.Value, operator logicalplan.Operator) (bool, error) {
	switch operator {
	case logicalplan.EqOp:
		bloomFilter := left.BloomFilter()
		if bloomFilter == nil {
			// If there is no bloom filter then we cannot make a statement about a
			// true negative so the rowgroup has to be a candidate.
			return true, nil
		}

		ok, err := bloomFilter.Check(right)
		if err != nil {
			return true, err
		}
		if !ok {
			// Bloom filters may return false positives, but never return false
			// negatives, we know this column chunk does not contain the value.
			return false, nil
		}
	}
	return true, nil
}
