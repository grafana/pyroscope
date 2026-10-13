package model

import (
	"cmp"
	"errors"
	"slices"
	"sync"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

var ErrProjectionRequiresV2 = errors.New("function projections require the v2 read path")

func IsProjectionFormat(format querierv1.ProfileFormat) bool {
	return format == querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS || format == querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE
}

func depthOrDefault(o *typesv1.FunctionTreeOptions) int {
	if o == nil || o.MaxDepth == nil {
		return 1
	}
	return int(*o.MaxDepth)
}

func EmptyFunctionTree(o *typesv1.FunctionTreeOptions) *typesv1.FunctionTree {
	t := new(typesv1.FunctionTree)
	if o.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS || o.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH {
		t.Callers = new(typesv1.CallTree)
	}
	if o.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES || o.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH {
		t.Callees = new(typesv1.CallTree)
	}
	return t
}

type FunctionTreeBuilder[N comparable] struct {
	callers *callTreeBuilder[N]
	callees *callTreeBuilder[N]
}

// NewFunctionTreeBuilder consumes options validated by the projection API boundary.
// ROOT_PATH input must already be selected. lookup maps the zero value to the
// empty synthetic-root name; other IDs are resolved only when Tree is called.
func NewFunctionTreeBuilder[N comparable](path []N, o *typesv1.FunctionTreeOptions, lookup func(N) string) *FunctionTreeBuilder[N] {
	b := new(FunctionTreeBuilder[N])
	if o.GetDirection() != typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES {
		b.callers = newCallTreeBuilder(path, depthOrDefault(o), typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, o.GetSelection(), lookup)
	}
	if o.GetDirection() != typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS {
		b.callees = newCallTreeBuilder(path, depthOrDefault(o), typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, o.GetSelection(), lookup)
	}
	return b
}

func (b *FunctionTreeBuilder[N]) Insert(stack []N, value int64) {
	if b.callers != nil {
		b.callers.Insert(stack, value)
	}
	if b.callees != nil {
		b.callees.Insert(stack, value)
	}
}

func (b *FunctionTreeBuilder[N]) Tree() *typesv1.FunctionTree {
	t := new(typesv1.FunctionTree)
	if b.callers != nil {
		t.Callers = b.callers.Tree()
	}
	if b.callees != nil {
		t.Callees = b.callees.Tree()
	}
	return t
}

type FunctionTreeMerger struct {
	mu      sync.Mutex
	callers *callTreeMerger
	callees *callTreeMerger
}

// NewFunctionTreeMerger preserves the requested directions even without matching samples.
// With nil options, directions are inferred from merged reports.
func NewFunctionTreeMerger(options *typesv1.FunctionTreeOptions) *FunctionTreeMerger {
	m := new(FunctionTreeMerger)
	if options.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS || options.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH {
		m.callers = new(callTreeMerger)
	}
	if options.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES || options.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH {
		m.callees = new(callTreeMerger)
	}
	return m
}

func (m *FunctionTreeMerger) Merge(t *typesv1.FunctionTree) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.GetCallers() != nil {
		if m.callers == nil {
			m.callers = new(callTreeMerger)
		}
		m.callers.merge(t.Callers)
	}
	if t.GetCallees() != nil {
		if m.callees == nil {
			m.callees = new(callTreeMerger)
		}
		m.callees.merge(t.Callees)
	}
}

func (m *FunctionTreeMerger) Tree() *typesv1.FunctionTree {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := new(typesv1.FunctionTree)
	if m.callers != nil {
		t.Callers = m.callers.tree()
	}
	if m.callees != nil {
		t.Callees = m.callees.tree()
	}
	return t
}

type FunctionTableMerger struct {
	mu        sync.Mutex
	total     int64
	functions map[string]*typesv1.FunctionStats
}

func NewFunctionTableMerger() *FunctionTableMerger {
	return &FunctionTableMerger{
		functions: make(map[string]*typesv1.FunctionStats),
	}
}

func (m *FunctionTableMerger) Merge(t *typesv1.FunctionTable) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total += t.GetTotal()
	for _, f := range t.GetFunctions() {
		dst := m.functions[f.Name]
		if dst == nil {
			dst = &typesv1.FunctionStats{Name: f.Name}
			m.functions[f.Name] = dst
		}
		dst.Total += f.Total
		dst.Self += f.Self
	}
}

func (m *FunctionTableMerger) Table() *typesv1.FunctionTable {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := &typesv1.FunctionTable{
		Total:          m.total,
		TotalFunctions: int64(len(m.functions)),
	}
	for _, f := range m.functions {
		t.Functions = append(t.Functions, f.CloneVT())
	}
	slices.SortFunc(t.Functions, func(a, b *typesv1.FunctionStats) int {
		if c := cmp.Compare(b.Self, a.Self); c != 0 {
			return c
		}
		if c := cmp.Compare(b.Total, a.Total); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return t
}
