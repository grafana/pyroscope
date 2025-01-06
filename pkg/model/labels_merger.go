package model

import (
	"slices"
	"sort"
	"sync"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type LabelMerger struct {
	mu     sync.Mutex
	names  map[string]struct{}
	values map[string]struct{}
	series map[uint64]*typesv1.Labels
}

func NewLabelMerger() *LabelMerger {
	return &LabelMerger{
		names:  make(map[string]struct{}),
		values: make(map[string]struct{}),
		series: make(map[uint64]*typesv1.Labels),
	}
}

func (m *LabelMerger) MergeLabelNames(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range names {
		m.names[n] = struct{}{}
	}
}

func (m *LabelMerger) MergeLabelValues(values []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range values {
		m.values[v] = struct{}{}
	}
}

func (m *LabelMerger) HasNames() bool {
	return len(m.names) > 0
}

func (m *LabelMerger) LabelNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := make([]string, len(m.names))
	var i int
	for n := range m.names {
		s[i] = n
		i++
	}
	sort.Strings(s)
	return s
}

func (m *LabelMerger) HasValues() bool {
	return len(m.values) > 0
}

func (m *LabelMerger) LabelValues() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := make([]string, len(m.values))
	var i int
	for v := range m.values {
		s[i] = v
		i++
	}
	sort.Strings(s)
	return s
}

func (m *LabelMerger) MergeSeries(series []*typesv1.Labels) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range series {
		m.series[Labels(s.Labels).Hash()] = s
	}
}

func (m *LabelMerger) MergeLabels(ls []Labels) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range ls {
		m.series[l.Hash()] = &typesv1.Labels{Labels: l}
	}
}

func (m *LabelMerger) Labels() []*typesv1.Labels {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := make([]*typesv1.Labels, len(m.series))
	var i int
	for _, v := range m.series {
		s[i] = v
		i++
	}
	slices.SortFunc(s, func(a, b *typesv1.Labels) int {
		return CompareLabelPairs(a.Labels, b.Labels)
	})
	return s
}

func (m *LabelMerger) HasSeries() bool {
	return len(m.series) > 0
}
