package model

import (
	"sort"
	"sync"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// SpanPresenceMerger sums sample totals without combining different units.
type SpanPresenceMerger struct {
	mu    sync.Mutex
	spans map[[2]string]*querierv1.SpanPresence
}

func (m *SpanPresenceMerger) Merge(spans []*querierv1.SpanPresence) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.spans == nil {
		m.spans = make(map[[2]string]*querierv1.SpanPresence)
	}
	for _, s := range spans {
		k := [2]string{s.TraceId, s.SpanId}
		dst := m.spans[k]
		if dst == nil {
			dst = &querierv1.SpanPresence{TraceId: s.TraceId, SpanId: s.SpanId}
			m.spans[k] = dst
		}
		for _, total := range s.Totals {
			var found bool
			for _, t := range dst.Totals {
				if t.ProfileTypeID == total.ProfileTypeID {
					t.Total += total.Total
					found = true
					break
				}
			}
			if !found {
				dst.Totals = append(dst.Totals, total.CloneVT())
			}
		}
	}
}

func (m *SpanPresenceMerger) Spans() []*querierv1.SpanPresence {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*querierv1.SpanPresence, 0, len(m.spans))
	for _, s := range m.spans {
		c := s.CloneVT()
		sort.Slice(c.Totals, func(i, j int) bool { return c.Totals[i].ProfileTypeID < c.Totals[j].ProfileTypeID })
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].TraceId != result[j].TraceId {
			return result[i].TraceId < result[j].TraceId
		}
		return result[i].SpanId < result[j].SpanId
	})
	return result
}
