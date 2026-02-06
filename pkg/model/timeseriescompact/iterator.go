package timeseriescompact

import (
	"github.com/grafana/pyroscope/pkg/iter"
)

type seriesIterator struct {
	series *compactSeries
	idx    int
	curr   CompactValue
}

func newSeriesIterator(s *compactSeries) *seriesIterator {
	return &seriesIterator{series: s, idx: -1}
}

func (it *seriesIterator) Next() bool {
	it.idx++
	if it.idx >= len(it.series.points) {
		return false
	}
	p := it.series.points[it.idx]
	it.curr = CompactValue{
		Ts:             p.Timestamp,
		SeriesKey:      it.series.key,
		SeriesRefs:     it.series.refs,
		Value:          p.Value,
		AnnotationRefs: p.AnnotationRefs,
		Exemplars:      p.Exemplars,
	}
	return true
}

func (it *seriesIterator) At() CompactValue { return it.curr }
func (it *seriesIterator) Err() error       { return nil }
func (it *seriesIterator) Close() error     { return nil }

// mergeIterator merges multiple series iterators by timestamp.
type mergeIterator struct {
	iters   []iter.Iterator[CompactValue]
	heads   []CompactValue
	valid   []bool
	current CompactValue
}

func newMergeIterator(iters []iter.Iterator[CompactValue]) *mergeIterator {
	m := &mergeIterator{
		iters: iters,
		heads: make([]CompactValue, len(iters)),
		valid: make([]bool, len(iters)),
	}
	for i, it := range iters {
		if it.Next() {
			m.heads[i] = it.At()
			m.valid[i] = true
		}
	}
	return m
}

func (m *mergeIterator) Next() bool {
	// Find minimum timestamp among valid heads
	minIdx := -1
	var minTs int64
	for i, v := range m.valid {
		if v && (minIdx == -1 || m.heads[i].Ts < minTs) {
			minIdx = i
			minTs = m.heads[i].Ts
		}
	}
	if minIdx == -1 {
		return false
	}

	m.current = m.heads[minIdx]

	// Advance the iterator we consumed from
	if m.iters[minIdx].Next() {
		m.heads[minIdx] = m.iters[minIdx].At()
	} else {
		m.valid[minIdx] = false
	}

	return true
}

func (m *mergeIterator) At() CompactValue { return m.current }
func (m *mergeIterator) Err() error       { return nil }
func (m *mergeIterator) Close() error {
	for _, it := range m.iters {
		it.Close()
	}
	return nil
}
