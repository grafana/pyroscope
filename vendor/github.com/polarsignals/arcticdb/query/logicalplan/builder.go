package logicalplan

import (
	"strings"

	"github.com/apache/arrow/go/v8/arrow"

	"github.com/polarsignals/arcticdb/dynparquet"
)

type Builder struct {
	plan *LogicalPlan
}

func (b Builder) Scan(
	provider TableProvider,
	tableName string,
) Builder {
	return Builder{
		plan: &LogicalPlan{
			TableScan: &TableScan{
				TableProvider: provider,
				TableName:     tableName,
			},
		},
	}
}

func (b Builder) ScanSchema(
	provider TableProvider,
	tableName string,
) Builder {
	return Builder{
		plan: &LogicalPlan{
			SchemaScan: &SchemaScan{
				TableProvider: provider,
				TableName:     tableName,
			},
		},
	}
}

func (b Builder) Project(
	exprs ...Expr,
) Builder {
	return Builder{
		plan: &LogicalPlan{
			Input: b.plan,
			Projection: &Projection{
				Exprs: exprs,
			},
		},
	}
}

type Visitor interface {
	PreVisit(expr Expr) bool
	PostVisit(expr Expr) bool
}

type StaticColumnMatcher struct {
	ColumnName string
}

func (m StaticColumnMatcher) Match(columnName string) bool {
	return m.ColumnName == columnName
}

type ColumnMatcher interface {
	Match(columnName string) bool
}

type DynamicColumnMatcher struct {
	ColumnName string
}

func (m DynamicColumnMatcher) Match(columnName string) bool {
	return strings.HasPrefix(columnName, m.ColumnName+".")
}

type Expr interface {
	DataType(*dynparquet.Schema) arrow.DataType
	Accept(Visitor) bool
	Name() string
	ColumnsUsed() []ColumnMatcher
	// Matcher returns a ColumnMatcher that can be used to identify a column by
	// a downstream plan. In contrast to the ColumnUsed function from the Expr
	// interface, it is not useful to identify which columns are to be read
	// physically. This is necessary to distinguish between projections.
	//
	// Take the example of a column that projects `XYZ > 0`. Matcher can be
	// used to identify the column in the resulting Apache Arrow frames, while
	// ColumnsUsed will return `XYZ` to be necessary to be loaded physically.
	Matcher() ColumnMatcher
}

func (b Builder) Filter(
	expr Expr,
) Builder {
	return Builder{
		plan: &LogicalPlan{
			Input: b.plan,
			Filter: &Filter{
				Expr: expr,
			},
		},
	}
}

func (b Builder) Distinct(
	columns ...Expr,
) Builder {
	return Builder{
		plan: &LogicalPlan{
			Input: b.plan,
			Distinct: &Distinct{
				Columns: columns,
			},
		},
	}
}

func (b Builder) Aggregate(
	aggExpr Expr,
	groupExprs ...Expr,
) Builder {
	return Builder{
		plan: &LogicalPlan{
			Input: b.plan,
			Aggregation: &Aggregation{
				GroupExprs: groupExprs,
				AggExpr:    aggExpr,
			},
		},
	}
}

func (b Builder) Build() *LogicalPlan {
	return b.plan
}
