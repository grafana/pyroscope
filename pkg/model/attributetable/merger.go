package attributetable

import (
	"sort"
	"sync"
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// SeriesMerger merges query.v1.Series with AttributeTable optimization.
type SeriesMerger struct {
	mu             sync.Mutex
	series         map[uint64]*queryv1.Series
	attributeTable *Table
	sum            bool
}

// NewSeriesMerger creates a new merger for query.v1.Series with AttributeTable.
func NewSeriesMerger(sum bool) *SeriesMerger {
	return &SeriesMerger{
		series:         make(map[uint64]*queryv1.Series),
		attributeTable: NewTable(),
		sum:            sum,
	}
}

// MergeWithAttributeTable merges series and remaps their attribute_refs to the unified AttributeTable.
func (m *SeriesMerger) MergeWithAttributeTable(series []*queryv1.Series, table *queryv1.AttributeTable) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build a mapping from old refs to new refs in the unified AttributeTable
	var refMap map[int64]int64
	if table != nil && len(table.Keys) > 0 {
		refMap = make(map[int64]int64, len(table.Keys))
		for i := range table.Keys {
			oldRef := int64(i)
			key := unique.Make(table.Keys[i])
			value := unique.Make(table.Values[i])
			newRef := m.attributeTable.LookupOrAdd(attributeKey{key: key, value: value})
			refMap[oldRef] = newRef
		}
	}

	// Fast path: no remapping needed
	if refMap == nil {
		for _, s := range series {
			h := hashLabels(s.Labels)
			existing, ok := m.series[h]
			if !ok {
				m.series[h] = s
				continue
			}
			existing.Points = append(existing.Points, s.Points...)
		}
		return
	}

	for _, s := range series {
		h := hashLabels(s.Labels)
		existing, ok := m.series[h]
		if !ok {
			// First time seeing this series - remap refs and store
			m.series[h] = m.remapSeriesRefs(s, refMap)
			continue
		}

		// Merge points from new series into existing series
		for _, newPoint := range s.Points {
			existing.Points = append(existing.Points, m.remapPointRefs(newPoint, refMap))
		}
	}
}

// remapSeriesRefs creates a copy of the series with remapped attribute_refs
func (m *SeriesMerger) remapSeriesRefs(s *queryv1.Series, refMap map[int64]int64) *queryv1.Series {
	points := make([]*queryv1.Point, len(s.Points))
	for i, p := range s.Points {
		points[i] = m.remapPointRefs(p, refMap)
	}

	return &queryv1.Series{
		Labels: s.Labels,
		Points: points,
	}
}

// remapPointRefs remaps attribute_refs in a single point
func (m *SeriesMerger) remapPointRefs(p *queryv1.Point, refMap map[int64]int64) *queryv1.Point {
	// Fast path: no exemplars
	if len(p.Exemplars) == 0 {
		return &queryv1.Point{
			Value:       p.Value,
			Timestamp:   p.Timestamp,
			Annotations: p.Annotations,
		}
	}

	exemplars := make([]*queryv1.Exemplar, len(p.Exemplars))
	for i, ex := range p.Exemplars {
		remappedRefs := make([]int64, len(ex.AttributeRefs))
		for j, oldRef := range ex.AttributeRefs {
			if newRef, ok := refMap[oldRef]; ok {
				remappedRefs[j] = newRef
			} else {
				// Shouldn't happen, but keep old ref if mapping is missing
				remappedRefs[j] = oldRef
			}
		}

		exemplars[i] = &queryv1.Exemplar{
			Timestamp:     ex.Timestamp,
			ProfileId:     ex.ProfileId,
			SpanId:        ex.SpanId,
			Value:         ex.Value,
			AttributeRefs: remappedRefs,
		}
	}

	return &queryv1.Point{
		Value:       p.Value,
		Timestamp:   p.Timestamp,
		Annotations: p.Annotations,
		Exemplars:   exemplars,
	}
}

// TimeSeries returns the merged series, sorted by labels.
func (m *SeriesMerger) TimeSeries() []*queryv1.Series {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*queryv1.Series, 0, len(m.series))
	for _, s := range m.series {
		s.Points = m.mergePoints(s.Points)
		result = append(result, s)
	}

	sort.Slice(result, func(i, j int) bool {
		return compareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})

	return result
}

// mergePoints merges points with the same timestamp
func (m *SeriesMerger) mergePoints(points []*queryv1.Point) []*queryv1.Point {
	if len(points) < 2 {
		return points
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp < points[j].Timestamp
	})

	if !m.sum {
		// No summing, just return sorted points
		return points
	}

	merged := make([]*queryv1.Point, 0, len(points))
	merged = append(merged, points[0])

	for i := 1; i < len(points); i++ {
		last := merged[len(merged)-1]
		if points[i].Timestamp == last.Timestamp {
			last.Value += points[i].Value
			last.Annotations = mergeAnnotations(last.Annotations, points[i].Annotations)
			last.Exemplars = mergeQueryExemplars(last.Exemplars, points[i].Exemplars)
		} else {
			merged = append(merged, points[i])
		}
	}

	return merged
}

// mergeQueryExemplars merges exemplars, keeping the highest value per profileID
func mergeQueryExemplars(a, b []*queryv1.Exemplar) []*queryv1.Exemplar {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	byProfileID := make(map[string]*queryv1.Exemplar, len(a)+len(b))
	for _, ex := range a {
		byProfileID[ex.ProfileId] = ex
	}

	for _, ex := range b {
		existing, found := byProfileID[ex.ProfileId]
		if !found {
			byProfileID[ex.ProfileId] = ex
		} else if ex.Value > existing.Value {
			// Keep the exemplar with higher value
			byProfileID[ex.ProfileId] = ex
		}
	}

	result := make([]*queryv1.Exemplar, 0, len(byProfileID))
	for _, ex := range byProfileID {
		result = append(result, ex)
	}

	// Sort by profile ID for consistent output
	sort.Slice(result, func(i, j int) bool {
		return result[i].ProfileId < result[j].ProfileId
	})

	return result
}

// AttributeTable returns the unified AttributeTable
func (m *SeriesMerger) AttributeTable() *Table {
	return m.attributeTable
}

// ExpandToFullLabels converts the merged query.v1.Series to types.v1.Series
// by expanding attribute_refs back to full labels.
func (m *SeriesMerger) ExpandToFullLabels() []*typesv1.Series {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*typesv1.Series, 0, len(m.series))
	for _, s := range m.series {
		hasExemplars := false
		for _, p := range s.Points {
			if len(p.Exemplars) > 0 {
				hasExemplars = true
				break
			}
		}

		points := make([]*typesv1.Point, len(s.Points))
		if !hasExemplars {
			for j, p := range s.Points {
				points[j] = &typesv1.Point{
					Value:       p.Value,
					Timestamp:   p.Timestamp,
					Annotations: p.Annotations,
				}
			}
		} else {
			for j, p := range s.Points {
				var exemplars []*typesv1.Exemplar
				if len(p.Exemplars) > 0 {
					exemplars = make([]*typesv1.Exemplar, len(p.Exemplars))
					for k, ex := range p.Exemplars {
						// Pre-allocate labels slice with exact size
						labels := make([]*typesv1.LabelPair, len(ex.AttributeRefs))
						labelCount := 0
						for _, ref := range ex.AttributeRefs {
							key, value, ok := m.attributeTable.GetKeyValue(ref)
							if ok {
								labels[labelCount] = &typesv1.LabelPair{
									Name:  key,
									Value: value,
								}
								labelCount++
							}
						}
						// Trim to actual count (in case some refs were invalid)
						labels = labels[:labelCount]

						exemplars[k] = &typesv1.Exemplar{
							Timestamp: ex.Timestamp,
							ProfileId: ex.ProfileId,
							SpanId:    ex.SpanId,
							Value:     ex.Value,
							Labels:    labels,
						}
					}
				}
				points[j] = &typesv1.Point{
					Value:       p.Value,
					Timestamp:   p.Timestamp,
					Annotations: p.Annotations,
					Exemplars:   exemplars,
				}
			}
		}
		result = append(result, &typesv1.Series{
			Labels: s.Labels,
			Points: points,
		})
	}

	// Sort series by labels
	sort.Slice(result, func(i, j int) bool {
		return compareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})

	return result
}
