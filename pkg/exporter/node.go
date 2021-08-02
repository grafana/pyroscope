package exporter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

// node represents expression to filter tree nodes.
type node struct {
	expr  *regexp.Regexp
	value func(*tree.Tree) (float64, bool)
}

// newNode creates a new node from the given expression.
//
// If the expression is "total", total tree samples count will be retrieved
// on "value" call. Otherwise, the expression must be a valid regexp which
// then will be used to filter tree nodes: sum of nodes "self" is returned
// on "value" call.
func newNode(expr string) (node, error) {
	var f node
	switch strings.ToLower(expr) {
	case "", "total":
		f.value = f.valueTotal
	default:
		nodeExpr, err := regexp.Compile(expr)
		if err != nil {
			return f, fmt.Errorf("node must be either 'total' or a valid regexp: %w", err)
		}
		f.expr = nodeExpr
		f.value = f.valueSelf
	}
	return f, nil
}

func (f node) valueTotal(t *tree.Tree) (float64, bool) {
	x := t.Samples()
	return float64(x), x != 0
}

func (f node) valueSelf(t *tree.Tree) (float64, bool) {
	var x uint64
	// TODO(kolesnikovae):
	//   instead of iterating through the tree every time,
	//   it may be better to lazily render tree to slice of
	//   strings that would be shared by all rules.
	t.Iterate(func(key []byte, self uint64) {
		if self > 0 && f.expr.Match(key[2:]) {
			x += self
		}
	})
	return float64(x), x != 0
}
