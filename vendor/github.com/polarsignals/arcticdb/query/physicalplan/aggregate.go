package physicalplan

import (
	"errors"
	"fmt"
	"hash/maphash"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/apache/arrow/go/v8/arrow/math"
	"github.com/apache/arrow/go/v8/arrow/memory"
	"github.com/apache/arrow/go/v8/arrow/scalar"
	"github.com/dgryski/go-metro"

	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/polarsignals/arcticdb/query/logicalplan"
)

func Aggregate(
	pool memory.Allocator,
	s *dynparquet.Schema,
	agg *logicalplan.Aggregation,
) (*HashAggregate, error) {
	groupByMatchers := make([]logicalplan.ColumnMatcher, 0, len(agg.GroupExprs))
	for _, groupExpr := range agg.GroupExprs {
		groupByMatchers = append(groupByMatchers, groupExpr.Matcher())
	}

	var (
		aggFunc      logicalplan.AggFunc
		aggFuncFound bool

		aggColumnMatcher logicalplan.ColumnMatcher
		aggColumnFound   bool
	)

	agg.AggExpr.Accept(PreExprVisitorFunc(func(expr logicalplan.Expr) bool {
		switch e := expr.(type) {
		case logicalplan.AggregationFunction:
			aggFunc = e.Func
			aggFuncFound = true
		case logicalplan.Column:
			aggColumnMatcher = e.Matcher()
			aggColumnFound = true
		}

		return true
	}))

	if !aggFuncFound {
		return nil, errors.New("aggregation function not found")
	}

	if !aggColumnFound {
		return nil, errors.New("aggregation column not found")
	}

	f, err := chooseAggregationFunction(aggFunc, agg.AggExpr.DataType(s))
	if err != nil {
		return nil, err
	}

	return NewHashAggregate(
		pool,
		agg.AggExpr.Name(),
		f,
		aggColumnMatcher,
		groupByMatchers,
	), nil
}

func chooseAggregationFunction(
	aggFunc logicalplan.AggFunc,
	dataType arrow.DataType,
) (AggregationFunction, error) {
	switch aggFunc {
	case logicalplan.SumAggFunc:
		switch dataType.ID() {
		case arrow.INT64:
			return &Int64SumAggregation{}, nil
		default:
			return nil, fmt.Errorf("unsupported sum of type: %s", dataType.Name())
		}
	default:
		return nil, fmt.Errorf("unsupported aggregation function: %s", aggFunc.String())
	}
}

type AggregationFunction interface {
	Aggregate(pool memory.Allocator, arrs []arrow.Array) (arrow.Array, error)
}

type HashAggregate struct {
	pool                  memory.Allocator
	resultColumnName      string
	groupByCols           map[string]array.Builder
	arraysToAggregate     []array.Builder
	hashToAggregate       map[uint64]int
	groupByColumnMatchers []logicalplan.ColumnMatcher
	columnToAggregate     logicalplan.ColumnMatcher
	aggregationFunction   AggregationFunction
	hashSeed              maphash.Seed
	nextCallback          func(r arrow.Record) error

	// Buffers that are reused across callback calls.
	groupByFields      []arrow.Field
	groupByFieldHashes []uint64
	groupByArrays      []arrow.Array
}

func NewHashAggregate(
	pool memory.Allocator,
	resultColumnName string,
	aggregationFunction AggregationFunction,
	columnToAggregate logicalplan.ColumnMatcher,
	groupByColumnMatchers []logicalplan.ColumnMatcher,
) *HashAggregate {
	return &HashAggregate{
		pool:              pool,
		resultColumnName:  resultColumnName,
		groupByCols:       map[string]array.Builder{},
		arraysToAggregate: make([]array.Builder, 0),
		hashToAggregate:   map[uint64]int{},
		columnToAggregate: columnToAggregate,
		// TODO: Matchers can be optimized to be something like a radix tree or just a fast-lookup datastructure for exact matches or prefix matches.
		groupByColumnMatchers: groupByColumnMatchers,
		hashSeed:              maphash.MakeSeed(),
		aggregationFunction:   aggregationFunction,

		groupByFields:      make([]arrow.Field, 0, 10),
		groupByFieldHashes: make([]uint64, 0, 10),
		groupByArrays:      make([]arrow.Array, 0, 10),
	}
}

func (a *HashAggregate) SetNextCallback(nextCallback func(r arrow.Record) error) {
	a.nextCallback = nextCallback
}

// Go translation of boost's hash_combine function. Read here why these values
// are used and good choices: https://stackoverflow.com/questions/35985960/c-why-is-boosthash-combine-the-best-way-to-combine-hash-values
func hashCombine(lhs, rhs uint64) uint64 {
	return lhs ^ (rhs + 0x9e3779b9 + (lhs << 6) + (lhs >> 2))
}

func hashArray(arr arrow.Array) []uint64 {
	switch arr.(type) {
	case *array.String:
		return hashStringArray(arr.(*array.String))
	case *array.Binary:
		return hashBinaryArray(arr.(*array.Binary))
	case *array.Int64:
		return hashInt64Array(arr.(*array.Int64))
	case *array.Boolean:
		return hashBooleanArray(arr.(*array.Boolean))
	default:
		panic("unsupported array type " + fmt.Sprintf("%T", arr))
	}
}

func hashBinaryArray(arr *array.Binary) []uint64 {
	res := make([]uint64, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		if !arr.IsNull(i) {
			res[i] = metro.Hash64(arr.Value(i), 0)
		}
	}
	return res
}

func hashBooleanArray(arr *array.Boolean) []uint64 {
	res := make([]uint64, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		if arr.IsNull(i) {
			res[i] = 0
			continue
		}
		if arr.Value(i) {
			res[i] = 2
		} else {
			res[i] = 1
		}
	}
	return res
}

func hashStringArray(arr *array.String) []uint64 {
	res := make([]uint64, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		if !arr.IsNull(i) {
			res[i] = metro.Hash64([]byte(arr.Value(i)), 0)
		}
	}
	return res
}

func hashInt64Array(arr *array.Int64) []uint64 {
	res := make([]uint64, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		if !arr.IsNull(i) {
			res[i] = uint64(arr.Value(i))
		}
	}
	return res
}

func (a *HashAggregate) Callback(r arrow.Record) error {
	groupByFields := a.groupByFields
	groupByFieldHashes := a.groupByFieldHashes
	groupByArrays := a.groupByArrays

	defer func() {
		groupByFields = groupByFields[:0]
		groupByFieldHashes = groupByFieldHashes[:0]
		groupByArrays = groupByArrays[:0]
	}()

	var columnToAggregate arrow.Array
	aggregateFieldFound := false

	for i, field := range r.Schema().Fields() {
		for _, matcher := range a.groupByColumnMatchers {
			if matcher.Match(field.Name) {
				groupByFields = append(groupByFields, field)
				groupByFieldHashes = append(groupByFieldHashes, scalar.Hash(a.hashSeed, scalar.NewStringScalar(field.Name)))
				groupByArrays = append(groupByArrays, r.Column(i))
			}
		}

		if a.columnToAggregate.Match(field.Name) {
			columnToAggregate = r.Column(i)
			aggregateFieldFound = true
		}
	}

	if !aggregateFieldFound {
		return errors.New("aggregate field not found, aggregations are not possible without it")
	}

	numRows := int(r.NumRows())

	colHashes := make([][]uint64, len(groupByArrays))
	for i, arr := range groupByArrays {
		colHashes[i] = hashArray(arr)
	}

	for i := 0; i < numRows; i++ {
		hash := uint64(0)
		for j := range colHashes {
			if colHashes[j][i] == 0 {
				continue
			}

			hash = hashCombine(
				hash,
				hashCombine(
					groupByFieldHashes[j],
					colHashes[j][i],
				),
			)
		}

		k, ok := a.hashToAggregate[hash]
		if !ok {
			agg := array.NewBuilder(a.pool, columnToAggregate.DataType())
			a.arraysToAggregate = append(a.arraysToAggregate, agg)
			k = len(a.arraysToAggregate) - 1
			a.hashToAggregate[hash] = k

			// insert new row into columns grouped by and create new aggregate array to append to.
			for j, arr := range groupByArrays {
				fieldName := groupByFields[j].Name

				groupByCol, found := a.groupByCols[fieldName]
				if !found {
					groupByCol = array.NewBuilder(a.pool, groupByFields[j].Type)
					a.groupByCols[fieldName] = groupByCol
				}

				// We already appended to the arrays to aggregate, so we have
				// to account for that. We only want to back-fill null values
				// up until the index that we are about to insert into.
				for groupByCol.Len() < len(a.arraysToAggregate)-1 {
					groupByCol.AppendNull()
				}

				err := appendValue(groupByCol, arr, i)
				if err != nil {
					return err
				}
			}
		}

		if err := appendValue(a.arraysToAggregate[k], columnToAggregate, i); err != nil {
			return err
		}
	}

	return nil
}

func appendValue(b array.Builder, arr arrow.Array, i int) error {
	if arr == nil || arr.IsNull(i) {
		b.AppendNull()
		return nil
	}

	switch arr := arr.(type) {
	case *array.Int64:
		b.(*array.Int64Builder).Append(arr.Value(i))
		return nil
	case *array.String:
		b.(*array.StringBuilder).Append(arr.Value(i))
		return nil
	case *array.Binary:
		b.(*array.BinaryBuilder).Append(arr.Value(i))
		return nil
	case *array.FixedSizeBinary:
		b.(*array.FixedSizeBinaryBuilder).Append(arr.Value(i))
		return nil
	case *array.Boolean:
		b.(*array.BooleanBuilder).Append(arr.Value(i))
		return nil
	// case *array.List:
	//	// TODO: This seems horribly inefficient, we already have the whole
	//	// array and are just doing an expensive copy, but arrow doesn't seem
	//	// to be able to append whole list scalars at once.
	//	length := s.Value.Len()
	//	larr := arr.(*array.ListBuilder)
	//	vb := larr.ValueBuilder()
	//	larr.Append(true)
	//	for i := 0; i < length; i++ {
	//		v, err := scalar.GetScalar(s.Value, i)
	//		if err != nil {
	//			return err
	//		}

	//		err = appendValue(vb, v)
	//		if err != nil {
	//			return err
	//		}
	//	}
	//	return nil
	default:
		return errors.New("unsupported type for arrow append")
	}
}

func (a *HashAggregate) Finish() error {
	numCols := len(a.groupByCols) + 1
	numRows := len(a.arraysToAggregate)

	groupByFields := make([]arrow.Field, 0, numCols)
	groupByArrays := make([]arrow.Array, 0, numCols)
	for fieldName, groupByCol := range a.groupByCols {
		for groupByCol.Len() < numRows {
			// It's possible that columns that are grouped by haven't occurred
			// in all aggregated rows which causes them to not be of equal size
			// as the total number of rows so we need to backfill. This happens
			// for example when there are different sets of dynamic columns in
			// different row-groups of the table.
			groupByCol.AppendNull()
		}
		arr := groupByCol.NewArray()
		groupByFields = append(groupByFields, arrow.Field{Name: fieldName, Type: arr.DataType()})
		groupByArrays = append(groupByArrays, arr)
	}

	arrs := make([]arrow.Array, 0, numRows)
	for _, arr := range a.arraysToAggregate {
		arrs = append(arrs, arr.NewArray())
	}

	aggregateArray, err := a.aggregationFunction.Aggregate(a.pool, arrs)
	if err != nil {
		return fmt.Errorf("aggregate batched arrays: %w", err)
	}

	aggregateField := arrow.Field{Name: a.resultColumnName, Type: aggregateArray.DataType()}
	cols := append(groupByArrays, aggregateArray)

	return a.nextCallback(array.NewRecord(
		arrow.NewSchema(append(groupByFields, aggregateField), nil),
		cols,
		int64(numRows),
	))
}

type Int64SumAggregation struct{}

var ErrUnsupportedSumType = errors.New("unsupported type for sum aggregation, expected int64")

func (a *Int64SumAggregation) Aggregate(pool memory.Allocator, arrs []arrow.Array) (arrow.Array, error) {
	if len(arrs) == 0 {
		return array.NewInt64Builder(pool).NewArray(), nil
	}

	typ := arrs[0].DataType().ID()
	switch typ {
	case arrow.INT64:
		return sumInt64arrays(pool, arrs), nil
	default:
		return nil, fmt.Errorf("sum array of %s: %w", typ, ErrUnsupportedSumType)
	}
}

func sumInt64arrays(pool memory.Allocator, arrs []arrow.Array) arrow.Array {
	res := array.NewInt64Builder(pool)
	for _, arr := range arrs {
		res.Append(sumInt64array(arr.(*array.Int64)))
	}

	return res.NewArray()
}

func sumInt64array(arr *array.Int64) int64 {
	return math.Int64.Sum(arr)
}
