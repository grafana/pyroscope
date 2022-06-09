package arcticdb

import (
	"errors"
	"fmt"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/scalar"
	"github.com/segmentio/parquet-go"

	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/polarsignals/arcticdb/query/logicalplan"
)

type PreExprVisitorFunc func(expr logicalplan.Expr) bool

func (f PreExprVisitorFunc) PreVisit(expr logicalplan.Expr) bool {
	return f(expr)
}

func (f PreExprVisitorFunc) PostVisit(expr logicalplan.Expr) bool {
	return false
}

type TrueNegativeFilter interface {
	Eval(dynparquet.DynamicRowGroup) (bool, error)
}

type AlwaysTrueFilter struct{}

func (f *AlwaysTrueFilter) Eval(dynparquet.DynamicRowGroup) (bool, error) {
	return true, nil
}

func arrowScalarToParquetValue(sc scalar.Scalar) (parquet.Value, error) {
	switch s := sc.(type) {
	case *scalar.String:
		return parquet.ValueOf(string(s.Data())), nil
	case *scalar.Int64:
		return parquet.ValueOf(s.Value), nil
	case *scalar.FixedSizeBinary:
		width := s.Type.(*arrow.FixedSizeBinaryType).ByteWidth
		v := [16]byte{}
		copy(v[:width], s.Data())
		return parquet.ValueOf(v), nil
	default:
		return parquet.Value{}, fmt.Errorf("unsupported scalar type %T", s)
	}
}

func binaryBooleanExpr(expr *logicalplan.BinaryExpr) (TrueNegativeFilter, error) {
	switch expr.Op {
	case logicalplan.EqOp: //, logicalplan.NotEqOp, logicalplan.LTOp, logicalplan.LTEOp, logicalplan.GTOp, logicalplan.GTEOp, logicalplan.RegExpOp, logicalplan.NotRegExpOp:
		var leftColumnRef *ColumnRef
		expr.Left.Accept(PreExprVisitorFunc(func(expr logicalplan.Expr) bool {
			switch e := expr.(type) {
			case *logicalplan.Column:
				leftColumnRef = &ColumnRef{
					ColumnName: e.ColumnName,
				}
				return false
			}
			return true
		}))
		if leftColumnRef == nil {
			return nil, errors.New("left side of binary expression must be a column")
		}

		var (
			rightValue parquet.Value
			err        error
		)
		expr.Right.Accept(PreExprVisitorFunc(func(expr logicalplan.Expr) bool {
			switch e := expr.(type) {
			case *logicalplan.LiteralExpr:
				rightValue, err = arrowScalarToParquetValue(e.Value)
				return false
			}
			return true
		}))

		if err != nil {
			return nil, err
		}

		return &BinaryScalarExpr{
			Left:  leftColumnRef,
			Op:    expr.Op,
			Right: rightValue,
		}, nil
	case logicalplan.AndOp:
		left, err := booleanExpr(expr.Left)
		if err != nil {
			return nil, err
		}

		right, err := booleanExpr(expr.Right)
		if err != nil {
			return nil, err
		}

		return &AndExpr{
			Left:  left,
			Right: right,
		}, nil
	default:
		return &AlwaysTrueFilter{}, nil
	}
}

type AndExpr struct {
	Left  TrueNegativeFilter
	Right TrueNegativeFilter
}

func (a *AndExpr) Eval(rg dynparquet.DynamicRowGroup) (bool, error) {
	left, err := a.Left.Eval(rg)
	if err != nil {
		return false, err
	}
	if !left {
		return false, nil
	}

	right, err := a.Right.Eval(rg)
	if err != nil {
		return false, err
	}

	// This stores the result in place to avoid allocations.
	return left && right, nil
}

func booleanExpr(expr logicalplan.Expr) (TrueNegativeFilter, error) {
	if expr == nil {
		return &AlwaysTrueFilter{}, nil
	}

	switch e := expr.(type) {
	case *logicalplan.BinaryExpr:
		return binaryBooleanExpr(e)
	default:
		return nil, fmt.Errorf("unsupported boolean expression %T", e)
	}
}
