package logicalplan

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/apache/arrow/go/v8/arrow/scalar"
	"github.com/segmentio/parquet-go/format"
)

// PlanValidationError is the error representing a logical plan that is not valid.
type PlanValidationError struct {
	message  string
	plan     *LogicalPlan
	children []*ExprValidationError
	input    *PlanValidationError
}

// PlanValidationError.Error prints the error message in a human-readable format.
// implements the error interface.
func (e *PlanValidationError) Error() string {
	message := make([]string, 0)
	message = append(message, e.message)
	message = append(message, "\n")
	message = append(message, fmt.Sprintf("%s", e.plan))

	for _, child := range e.children {
		message = append(message, "\n  -> invalid expression: ")
		message = append(message, child.Error())
		message = append(message, "\n")
	}

	if e.input != nil {
		message = append(message, "-> invalid input: ")
		message = append(message, e.input.Error())
	}

	return strings.Join(message, "")
}

// ExprValidationError is the error for an invalid expression that was found during validation.
type ExprValidationError struct {
	message  string
	expr     Expr
	children []*ExprValidationError
}

// ExprValidationError.Error prints the error message in a human-readable format.
// implements the error interface.
func (e *ExprValidationError) Error() string {
	message := make([]string, 0)
	message = append(message, e.message)
	message = append(message, ": ")
	message = append(message, fmt.Sprintf("%s", e.expr))
	for _, child := range e.children {
		message = append(message, "\n     -> invalid sub-expression: ")
		message = append(message, child.Error())
	}

	return strings.Join(message, "")
}

// Validate validates the logical plan.
func Validate(plan *LogicalPlan) error {
	err := ValidateSingleFieldSet(plan)
	if err == nil {
		switch {
		case plan.SchemaScan != nil:
			err = nil
		case plan.TableScan != nil:
			err = nil
		case plan.Filter != nil:
			err = ValidateFilter(plan)
		case plan.Distinct != nil:
			err = nil
		case plan.Projection != nil:
			err = nil
		case plan.Aggregation != nil:
			err = nil
		}
	}

	// traverse backwards up the plan to validate all inputs
	inputErr := ValidateInput(plan)
	if inputErr != nil {
		if err == nil {
			err = inputErr
		} else {
			err.input = inputErr
		}
	}

	if err != nil {
		return err
	}
	return nil
}

// ValidateSingleFieldSet checks that only a single field is set on the plan.
func ValidateSingleFieldSet(plan *LogicalPlan) *PlanValidationError {
	fieldsSet := make([]int, 0)
	if plan.SchemaScan != nil {
		fieldsSet = append(fieldsSet, 0)
	}
	if plan.TableScan != nil {
		fieldsSet = append(fieldsSet, 1)
	}
	if plan.Filter != nil {
		fieldsSet = append(fieldsSet, 2)
	}
	if plan.Distinct != nil {
		fieldsSet = append(fieldsSet, 3)
	}
	if plan.Projection != nil {
		fieldsSet = append(fieldsSet, 4)
	}
	if plan.Aggregation != nil {
		fieldsSet = append(fieldsSet, 5)
	}

	if len(fieldsSet) != 1 {
		fieldsFound := make([]string, 0)
		fields := []string{"SchemaScan", "TableScan", "Filter", "Distinct", "Projection", "Aggregation"}
		for _, i := range fieldsSet {
			fieldsFound = append(fieldsFound, fields[i])
		}

		message := make([]string, 0)
		message = append(message,
			fmt.Sprintf("invalid number of fields. expected: 1, found: %d (%s). plan must only have one of the following: ",
				len(fieldsSet),
				strings.Join(fieldsFound, ", "),
			),
		)
		message = append(message, strings.Join(fields, ", "))

		return &PlanValidationError{
			plan:    plan,
			message: strings.Join(message, ""),
		}
	}
	return nil
}

// ValidateInput validates that the current logical plans input is valid.
// It returns nil if the plan has no input.
func ValidateInput(plan *LogicalPlan) *PlanValidationError {
	if plan.Input != nil {
		inputErr := Validate(plan.Input)
		if inputErr != nil {
			inputValidationErr, ok := inputErr.(*PlanValidationError)
			if !ok {
				// if we are here it is a bug in the code
				panic(fmt.Sprintf("Unexpected error: %v expected a PlanValidationError", inputErr))
			}
			return inputValidationErr
		}
	}
	return nil
}

// ValidateFilter validates the logical plan's filter step.
func ValidateFilter(plan *LogicalPlan) *PlanValidationError {
	if err := ValidateFilterExpr(plan, plan.Filter.Expr); err != nil {
		return &PlanValidationError{
			message:  "invalid filter",
			plan:     plan,
			children: []*ExprValidationError{err},
		}
	}
	return nil
}

// ValidateFilterExpr validates filter's expression.
func ValidateFilterExpr(plan *LogicalPlan, e Expr) *ExprValidationError {
	switch expr := e.(type) {
	case *BinaryExpr:
		err := ValidateFilterBinaryExpr(plan, expr)
		return err
	}

	return nil
}

// ValidateFilterBinaryExpr validates the filter's binary expression.
func ValidateFilterBinaryExpr(plan *LogicalPlan, expr *BinaryExpr) *ExprValidationError {
	if expr.Op == AndOp {
		return ValidateFilterAndBinaryExpr(plan, expr)
	}

	// try to find the column expression on the left side of the binary expression
	leftColumnFinder := newTypeFinder((*Column)(nil))
	expr.Left.Accept(&leftColumnFinder)
	if leftColumnFinder.result == nil {
		return &ExprValidationError{
			message: "left side of binary expression must be a column",
			expr:    expr,
		}
	}

	// try to find the column in the schema
	columnExpr := leftColumnFinder.result.(*Column)
	schema := plan.InputSchema()
	if schema != nil {
		column, found := schema.ColumnByName(columnExpr.ColumnName)
		if found {
			// try to find the literal on the other side of the expression
			rightLiteralFinder := newTypeFinder((*LiteralExpr)(nil))
			expr.Right.Accept(&rightLiteralFinder)
			if rightLiteralFinder.result != nil {
				// ensure that the column type is compatible with the literal being compared to it
				t := column.StorageLayout.Type()
				literalExpr := rightLiteralFinder.result.(*LiteralExpr)
				if err := ValidateComparingTypes(t.LogicalType(), literalExpr.Value); err != nil {
					err.expr = expr
					return err
				}
			}
		}
	}

	return nil
}

// ValidateComparingTypes validates if the types being compared by a binary expression are compatible.
func ValidateComparingTypes(columnType *format.LogicalType, literal scalar.Scalar) *ExprValidationError {
	switch {
	// if the column is a string type, it shouldn't be compared to a number
	case columnType.UTF8 != nil:
		switch literal.(type) {
		case *scalar.Float64:
			return &ExprValidationError{
				message: "incompatible types: string column cannot be compared with numeric literal",
			}
		case *scalar.Int64:
			return &ExprValidationError{
				message: "incompatible types: string column cannot be compared with numeric literal",
			}
		}
	// if the column is a numeric type, it shouldn't be compared to a string
	case columnType.Integer != nil:
		switch literal.(type) {
		case *scalar.String:
			return &ExprValidationError{
				message: "incompatible types: numeric column cannot be compared with string literal",
			}
		}
	}
	return nil
}

// ValidateFilterAndBinaryExpr validates the filter's binary expression where Op = AND.
func ValidateFilterAndBinaryExpr(plan *LogicalPlan, expr *BinaryExpr) *ExprValidationError {
	leftErr := ValidateFilterExpr(plan, expr.Left)
	rightErr := ValidateFilterExpr(plan, expr.Right)

	if leftErr != nil || rightErr != nil {
		message := make([]string, 0, 3)
		message = append(message, "invalid children:")

		validationErr := ExprValidationError{
			expr:     expr,
			children: make([]*ExprValidationError, 0),
		}

		if leftErr != nil {
			lve := leftErr
			message = append(message, "left")
			validationErr.children = append(validationErr.children, lve)
		}

		if rightErr != nil {
			lve := rightErr
			message = append(message, "right")
			validationErr.children = append(validationErr.children, lve)
		}

		validationErr.message = strings.Join(message, " ")
		return &validationErr
	}
	return nil
}

// NewTypeFinder returns an instance of the findExpressionForTypeVisitor for the
// passed type. It expects to receive a pointer to the  type it is will find.
func newTypeFinder(val interface{}) findExpressionForTypeVisitor {
	return findExpressionForTypeVisitor{exprType: reflect.TypeOf(val)}
}

// findExpressionForTypeVisitor is an instance of Visitor that will try to find
// an expression of the given type while visiting the expressions.
type findExpressionForTypeVisitor struct {
	exprType reflect.Type
	// if an expression of the type is found, it will be set on this field after
	// visiting. Other-wise this field will be nil
	result Expr
}

func (v *findExpressionForTypeVisitor) PreVisit(expr Expr) bool {
	return true
}

func (v *findExpressionForTypeVisitor) PostVisit(expr Expr) bool {
	found := v.exprType == reflect.TypeOf(expr)
	if found {
		v.result = expr
	}
	return !found
}
