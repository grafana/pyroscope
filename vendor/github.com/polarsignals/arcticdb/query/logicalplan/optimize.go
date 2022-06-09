package logicalplan

type Optimizer interface {
	Optimize(plan *LogicalPlan) *LogicalPlan
}

var DefaultOptimizers = []Optimizer{
	&PhysicalProjectionPushDown{},
	&FilterPushDown{},
	&DistinctPushDown{},
	&ProjectionPushDown{},
}

// The PhysicalProjectionPushDown optimizer tries to push down the actual
// physical columns used by the query to the table scan, so the table provider
// can decide to only read the columns that are actually going to be used by
// the query.
type PhysicalProjectionPushDown struct{}

func (p *PhysicalProjectionPushDown) Optimize(plan *LogicalPlan) *LogicalPlan {
	p.optimize(plan, nil)
	return plan
}

func (p *PhysicalProjectionPushDown) optimize(plan *LogicalPlan, columnsUsed []ColumnMatcher) {
	switch {
	case plan.SchemaScan != nil:
		plan.SchemaScan.Projection = columnsUsed
	case plan.TableScan != nil:
		plan.TableScan.Projection = columnsUsed
	case plan.Filter != nil:
		columnsUsed = append(columnsUsed, plan.Filter.Expr.ColumnsUsed()...)
	case plan.Distinct != nil:
		for _, expr := range plan.Distinct.Columns {
			columnsUsed = append(columnsUsed, expr.ColumnsUsed()...)
		}
	case plan.Projection != nil:
		for _, expr := range plan.Projection.Exprs {
			columnsUsed = append(columnsUsed, expr.ColumnsUsed()...)
		}
	case plan.Aggregation != nil:
		for _, expr := range plan.Aggregation.GroupExprs {
			columnsUsed = append(columnsUsed, expr.ColumnsUsed()...)
		}
		columnsUsed = append(columnsUsed, plan.Aggregation.AggExpr.ColumnsUsed()...)
	}

	if plan.Input != nil {
		p.optimize(plan.Input, columnsUsed)
	}
}

// The ProjectionPushDown finds the projection expressions that can be pushed
// down. If there is no projection expression, but there is an implicit
// projection such as a `Distinct` query plan, then it will insert a new
// projection plan and push it down. It functions in three steps, first it will
// find the projection expressions in the plan, then remove explicit projection
// plans from the overall plan if it exists, and will then synthesize one if it
// doesn't exist, and insert it in the deepest possible position in the plan.
type ProjectionPushDown struct{}

func (p *ProjectionPushDown) Optimize(plan *LogicalPlan) *LogicalPlan {
	c := &projectionCollector{}
	c.collect(plan)

	if len(c.projections) == 0 {
		// If there are no projection expressions, then we don't need to do
		// anything.
		return plan
	}

	plan = removeProjection(plan)
	return insertProjection(plan, &Projection{Exprs: c.projections})
}

type projectionCollector struct {
	projections []Expr
}

func (p *projectionCollector) collect(plan *LogicalPlan) {
	switch {
	case plan.Distinct != nil:
		p.projections = append(p.projections, plan.Distinct.Columns...)
	case plan.Projection != nil:
		p.projections = append(p.projections, plan.Projection.Exprs...)
	}

	if plan.Input != nil {
		p.collect(plan.Input)
	}
}

func removeProjection(plan *LogicalPlan) *LogicalPlan {
	if plan == nil {
		return nil
	}

	switch {
	case plan.Projection != nil:
		return plan.Input
	}

	plan.Input = removeProjection(plan.Input)
	return plan
}

func insertProjection(cur *LogicalPlan, projection *Projection) *LogicalPlan {
	if cur == nil {
		return nil
	}

	switch {
	case cur.TableScan != nil:
		return &LogicalPlan{
			Input:      cur,
			Projection: projection,
		}
	case cur.SchemaScan != nil:
		return &LogicalPlan{
			Input:      cur,
			Projection: projection,
		}
	}

	cur.Input = insertProjection(cur.Input, projection)
	return cur
}

// The FilterPushDown optimizer tries to push down the filters of a query down
// to the actual physical table scan. This allows the table provider to make
// smarter decisions about which pieces of data to load in the first place or
// which are definitely not useful to the query at all. It does not guarantee
// that all data will be filtered accordingly, it is just a mechanism to read
// less data from disk. It modifies the plan in place.
type FilterPushDown struct{}

func (p *FilterPushDown) Optimize(plan *LogicalPlan) *LogicalPlan {
	p.optimize(plan, nil)
	return plan
}

func (p *FilterPushDown) optimize(plan *LogicalPlan, exprs []Expr) {
	switch {
	case plan.SchemaScan != nil:
		if len(exprs) > 0 {
			plan.SchemaScan.Filter = and(exprs)
		}
	case plan.TableScan != nil:
		if len(exprs) > 0 {
			plan.TableScan.Filter = and(exprs)
		}
	case plan.Filter != nil:
		exprs = append(exprs, plan.Filter.Expr)
	}

	if plan.Input != nil {
		p.optimize(plan.Input, exprs)
	}
}

// The DistinctPushDown optimizer tries to push down the distinct operator to
// the table provider. There are certain cases of distinct queries where the
// storage engine can make smarter decisions than just returning all the data,
// such as with dictionary encoded columns that are not filtered they can
// return only the dictionary avoiding unnecessary decoding and deduplication
// in downstream distinct operators. It modifies the plan in place.
type DistinctPushDown struct{}

func (p *DistinctPushDown) Optimize(plan *LogicalPlan) *LogicalPlan {
	p.optimize(plan, nil)
	return plan
}

func (p *DistinctPushDown) optimize(plan *LogicalPlan, distinctColumns []ColumnMatcher) {
	switch {
	case plan.TableScan != nil:
		if len(distinctColumns) > 0 {
			plan.TableScan.Distinct = distinctColumns
		}
	case plan.Distinct != nil:
		for _, expr := range plan.Distinct.Columns {
			distinctColumns = append(distinctColumns, expr.Matcher())
		}
	}

	if plan.Input != nil {
		p.optimize(plan.Input, distinctColumns)
	}
}
