package timeseriescompact

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
)

// Merger merges time series reports in compact format.
type Merger struct {
	mu       sync.Mutex
	atMerger *attributetable.Merger
	series   map[string]*compactSeries
}

type compactSeries struct {
	key    string
	refs   []int64
	points []*queryv1.Point
}

// NewMerger creates a new compact time series merger.
func NewMerger() *Merger {
	return &Merger{
		atMerger: attributetable.NewMerger(),
		series:   make(map[string]*compactSeries),
	}
}

// MergeReport adds a report to the merger, remapping attribute refs.
func (m *Merger) MergeReport(r *queryv1.TimeSeriesCompactReport) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r == nil || len(r.TimeSeries) == 0 {
		return
	}

	m.atMerger.Merge(r.AttributeTable, func(remap *attributetable.Remapper) {
		for _, s := range r.TimeSeries {
			refs := remap.Refs(s.AttributeRefs)
			key := seriesKey(refs)

			existing, ok := m.series[key]
			if !ok {
				existing = &compactSeries{key: key, refs: refs}
				m.series[key] = existing
			}

			existing.points = slices.Grow(existing.points, len(s.Points))
			for _, p := range s.Points {
				pt := &queryv1.Point{Timestamp: p.Timestamp, Value: p.Value}
				if len(p.AnnotationRefs) > 0 {
					pt.AnnotationRefs = remap.Refs(p.AnnotationRefs)
				}
				if len(p.Exemplars) > 0 {
					pt.Exemplars = make([]*queryv1.Exemplar, len(p.Exemplars))
					for i, ex := range p.Exemplars {
						pt.Exemplars[i] = &queryv1.Exemplar{
							Timestamp:     ex.Timestamp,
							ProfileId:     ex.ProfileId,
							SpanId:        ex.SpanId,
							Value:         ex.Value,
							AttributeRefs: remap.Refs(ex.AttributeRefs),
						}
					}
				}
				existing.points = append(existing.points, pt)
			}
		}
	})
}

// Iterator returns an iterator over all merged series.
func (m *Merger) Iterator() iter.Iterator[CompactValue] {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.series) == 0 {
		return iter.NewEmptyIterator[CompactValue]()
	}

	// Sort series keys for deterministic output
	keys := make([]string, 0, len(m.series))
	for k := range m.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create iterators for each series
	iters := make([]iter.Iterator[CompactValue], 0, len(keys))
	for _, k := range keys {
		s := m.series[k]
		sort.Slice(s.points, func(i, j int) bool {
			return s.points[i].Timestamp < s.points[j].Timestamp
		})
		iters = append(iters, newSeriesIterator(s))
	}

	return newMergeIterator(iters)
}

// BuildAttributeTable returns the merged attribute table.
func (m *Merger) BuildAttributeTable() *queryv1.AttributeTable {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.atMerger.BuildAttributeTable(nil)
}

func seriesKey(refs []int64) string {
	var sb strings.Builder
	for i, ref := range refs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatInt(ref, 16))
	}
	return sb.String()
}
