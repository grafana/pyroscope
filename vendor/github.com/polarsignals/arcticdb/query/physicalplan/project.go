package physicalplan

import (
	"fmt"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/apache/arrow/go/v8/arrow/memory"

	"github.com/polarsignals/arcticdb/query/logicalplan"
)

type columnProjection interface {
	Project(mem memory.Allocator, ar arrow.Record) (arrow.Field, arrow.Array, error)
}

type aliasProjection struct {
	matcher logicalplan.ColumnMatcher
	name    string
}

func (a aliasProjection) Project(mem memory.Allocator, ar arrow.Record) (arrow.Field, arrow.Array, error) {
	for i, field := range ar.Schema().Fields() {
		if a.matcher.Match(field.Name) {
			field.Name = a.name
			return field, ar.Column(i), nil
		}
	}

	return arrow.Field{}, nil, nil
}

type binaryExprProjection struct {
	boolExpr BooleanExpression
}

func (b binaryExprProjection) Project(mem memory.Allocator, ar arrow.Record) (arrow.Field, arrow.Array, error) {
	bitmap, err := b.boolExpr.Eval(ar)
	if err != nil {
		return arrow.Field{}, nil, err
	}

	vals := make([]bool, ar.NumRows())
	builder := array.NewBooleanBuilder(mem)

	// We can do this because we now the values in the array are between 0 and
	// NumRows()-1
	for _, pos := range bitmap.ToArray() {
		vals[int(pos)] = true
	}

	builder.AppendValues(vals, nil)

	return arrow.Field{
		Name: b.boolExpr.String(),
		Type: &arrow.BooleanType{},
	}, builder.NewArray(), nil
}

type plainProjection struct {
	matcher logicalplan.ColumnMatcher
}

func (p plainProjection) Project(mem memory.Allocator, ar arrow.Record) (arrow.Field, arrow.Array, error) {
	for i, field := range ar.Schema().Fields() {
		if p.matcher.Match(field.Name) {
			return field, ar.Column(i), nil
		}
	}

	return arrow.Field{}, nil, nil
}

func projectionFromExpr(expr logicalplan.Expr) (columnProjection, error) {
	switch e := expr.(type) {
	case logicalplan.Column:
		return plainProjection{
			matcher: e.Matcher(),
		}, nil
	case logicalplan.AliasExpr:
		return aliasProjection{
			matcher: e.Matcher(),
			name:    e.Name(),
		}, nil
	case logicalplan.BinaryExpr:
		boolExpr, err := binaryBooleanExpr(e)
		if err != nil {
			return nil, err
		}
		return binaryExprProjection{boolExpr: boolExpr}, nil
	default:
		return nil, fmt.Errorf("unsupported expression type for projection: %T", expr)
	}
}

type Projection struct {
	pool           memory.Allocator
	colProjections []columnProjection

	next func(r arrow.Record) error
}

func Project(mem memory.Allocator, exprs []logicalplan.Expr) (*Projection, error) {
	p := &Projection{
		pool:           mem,
		colProjections: make([]columnProjection, 0, len(exprs)),
	}

	for _, e := range exprs {
		proj, err := projectionFromExpr(e)
		if err != nil {
			return nil, err
		}
		p.colProjections = append(p.colProjections, proj)
	}

	return p, nil
}

func (p *Projection) Callback(r arrow.Record) error {
	resFields := make([]arrow.Field, 0, len(p.colProjections))
	resArrays := make([]arrow.Array, 0, len(p.colProjections))

	for _, proj := range p.colProjections {
		f, a, err := proj.Project(p.pool, r)
		if err != nil {
			return err
		}
		if a == nil {
			continue
		}

		resFields = append(resFields, f)
		resArrays = append(resArrays, a)
	}

	rows := int64(0)
	if len(resArrays) > 0 {
		rows = int64(resArrays[0].Len())
	}

	ar := array.NewRecord(
		arrow.NewSchema(resFields, nil),
		resArrays,
		rows,
	)
	return p.next(ar)
}

func (p *Projection) SetNextCallback(next func(r arrow.Record) error) {
	p.next = next
}
