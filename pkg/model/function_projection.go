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
	return format == querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS
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
