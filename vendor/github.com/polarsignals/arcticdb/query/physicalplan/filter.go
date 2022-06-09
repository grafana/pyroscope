package physicalplan

import (
	"errors"
	"regexp"

	"github.com/RoaringBitmap/roaring"
	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/apache/arrow/go/v8/arrow/memory"
	"github.com/apache/arrow/go/v8/arrow/scalar"

	"github.com/polarsignals/arcticdb/query/logicalplan"
)

type PredicateFilter struct {
	pool         memory.Allocator
	filterExpr   BooleanExpression
	nextCallback func(r arrow.Record) error
}

type Bitmap = roaring.Bitmap

func NewBitmap() *Bitmap {
	return roaring.New()
}

type BooleanExpression interface {
	Eval(r arrow.Record) (*Bitmap, error)
	String() string
}

var ErrUnsupportedBooleanExpression = errors.New("unsupported boolean expression")

type ArrayReference struct{}

type PreExprVisitorFunc func(expr logicalplan.Expr) bool

func (f PreExprVisitorFunc) PreVisit(expr logicalplan.Expr) bool {
	return f(expr)
}

func (f PreExprVisitorFunc) PostVisit(expr logicalplan.Expr) bool {
	return false
}

func binaryBooleanExpr(expr logicalplan.BinaryExpr) (BooleanExpression, error) {
	switch expr.Op {
	case logicalplan.EqOp, logicalplan.NotEqOp, logicalplan.LTOp, logicalplan.LTEOp, logicalplan.GTOp, logicalplan.GTEOp, logicalplan.RegExpOp, logicalplan.NotRegExpOp:
		var leftColumnRef *ArrayRef
		expr.Left.Accept(PreExprVisitorFunc(func(expr logicalplan.Expr) bool {
			switch e := expr.(type) {
			case logicalplan.Column:
				leftColumnRef = &ArrayRef{
					ColumnName: e.ColumnName,
				}
				return false
			}
			return true
		}))
		if leftColumnRef == nil {
			return nil, errors.New("left side of binary expression must be a column")
		}

		var rightScalar scalar.Scalar
		expr.Right.Accept(PreExprVisitorFunc(func(expr logicalplan.Expr) bool {
			switch e := expr.(type) {
			case logicalplan.LiteralExpr:
				rightScalar = e.Value
				return false
			}
			return true
		}))

		switch expr.Op {
		case logicalplan.RegExpOp:
			regexp, err := regexp.Compile(string(rightScalar.(*scalar.String).Data()))
			if err != nil {
				return nil, err
			}
			return &RegExpFilter{
				left:  leftColumnRef,
				right: regexp,
			}, nil
		case logicalplan.NotRegExpOp:
			regexp, err := regexp.Compile(string(rightScalar.(*scalar.String).Data()))
			if err != nil {
				return nil, err
			}
			return &RegExpFilter{
				left:     leftColumnRef,
				right:    regexp,
				notMatch: true,
			}, nil
		}

		return &BinaryScalarExpr{
			Left:  leftColumnRef,
			Op:    expr.Op,
			Right: rightScalar,
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
		panic("unsupported binary boolean expression")
	}
}

type AndExpr struct {
	Left  BooleanExpression
	Right BooleanExpression
}

func (a *AndExpr) Eval(r arrow.Record) (*Bitmap, error) {
	left, err := a.Left.Eval(r)
	if err != nil {
		return nil, err
	}

	right, err := a.Right.Eval(r)
	if err != nil {
		return nil, err
	}

	// This stores the result in place to avoid allocations.
	left.And(right)
	return left, nil
}

func (a *AndExpr) String() string {
	return "(" + a.Left.String() + " AND " + a.Right.String() + ")"
}

func booleanExpr(expr logicalplan.Expr) (BooleanExpression, error) {
	switch e := expr.(type) {
	case logicalplan.BinaryExpr:
		return binaryBooleanExpr(e)
	default:
		return nil, ErrUnsupportedBooleanExpression
	}
}

func Filter(pool memory.Allocator, filterExpr logicalplan.Expr) (*PredicateFilter, error) {
	expr, err := booleanExpr(filterExpr)
	if err != nil {
		return nil, err
	}

	return newFilter(pool, expr), nil
}

func newFilter(pool memory.Allocator, filterExpr BooleanExpression) *PredicateFilter {
	return &PredicateFilter{
		pool:       pool,
		filterExpr: filterExpr,
	}
}

func (f *PredicateFilter) SetNextCallback(callback func(r arrow.Record) error) {
	f.nextCallback = callback
}

func (f *PredicateFilter) Callback(r arrow.Record) error {
	filtered, empty, err := filter(f.pool, f.filterExpr, r)
	if err != nil {
		return err
	}
	if empty {
		return nil
	}

	defer filtered.Release()
	return f.nextCallback(filtered)
}

func filter(pool memory.Allocator, filterExpr BooleanExpression, ar arrow.Record) (arrow.Record, bool, error) {
	bitmap, err := filterExpr.Eval(ar)
	if err != nil {
		return nil, true, err
	}

	if bitmap.IsEmpty() {
		return nil, true, nil
	}

	indicesToKeep := bitmap.ToArray()
	ranges := buildIndexRanges(indicesToKeep)

	totalRows := int64(0)
	recordRanges := make([]arrow.Record, len(ranges))
	for j, r := range ranges {
		recordRanges[j] = ar.NewSlice(int64(r.Start), int64(r.End))
		totalRows += int64(r.End - r.Start)
	}

	cols := make([]arrow.Array, ar.NumCols())
	numRanges := len(recordRanges)
	for i := range cols {
		colRanges := make([]arrow.Array, 0, numRanges)
		for _, rr := range recordRanges {
			colRanges = append(colRanges, rr.Column(i))
		}

		cols[i], err = array.Concatenate(colRanges, pool)
		if err != nil {
			return nil, true, err
		}
	}

	return array.NewRecord(ar.Schema(), cols, totalRows), false, nil
}

type IndexRange struct {
	Start uint32
	End   uint32
}

// buildIndexRanges returns a set of continguous index ranges from the given indicies
// ex: [1,2,7,8,9] would return [{Start:1, End:2},{Start:7,End:9}]
func buildIndexRanges(indices []uint32) []IndexRange {
	ranges := []IndexRange{}

	cur := IndexRange{
		Start: indices[0],
		End:   indices[0] + 1,
	}

	for _, i := range indices[1:] {
		if i == cur.End {
			cur.End++
		} else {
			ranges = append(ranges, cur)
			cur = IndexRange{
				Start: i,
				End:   i + 1,
			}
		}
	}

	ranges = append(ranges, cur)
	return ranges
}
