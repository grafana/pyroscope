package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// SeriesMerger merges query.v1.Series with AttributeTable optimization.
type SeriesMerger struct {
	mu             sync.Mutex
	series         map[string]*queryv1.Series
	attributeTable *Table
	sum            bool
}

// NewSeriesMerger creates a new merger for query.v1.Series with AttributeTable.
func NewSeriesMerger(sum bool) *SeriesMerger {
	return &SeriesMerger{
		series:         make(map[string]*queryv1.Series),
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
			key := seriesKey(s.AttributeRefs)
			existing, ok := m.series[key]
			if !ok {
				m.series[key] = s
				continue
			}
			existing.Points = append(existing.Points, s.Points...)
		}
		return
	}

	for _, s := range series {
		// Remap series attribute_refs first
		remappedRefs := m.remapRefs(s.AttributeRefs, refMap)
		key := seriesKey(remappedRefs)
		existing, ok := m.series[key]
		if !ok {
			// First time seeing this series - remap refs and store
			m.series[key] = m.remapSeriesRefs(s, refMap, remappedRefs)
			continue
		}

		// Merge points from new series into existing series
		for _, newPoint := range s.Points {
			existing.Points = append(existing.Points, m.remapPointRefs(newPoint, refMap))
		}
	}
}

// remapSeriesRefs creates a copy of the series with remapped attribute_refs
func (m *SeriesMerger) remapSeriesRefs(s *queryv1.Series, refMap map[int64]int64, remappedSeriesRefs []int64) *queryv1.Series {
	points := make([]*queryv1.Point, len(s.Points))
	for i, p := range s.Points {
		points[i] = m.remapPointRefs(p, refMap)
	}

	return &queryv1.Series{
		AttributeRefs: remappedSeriesRefs,
		Points:        points,
	}
}

// remapRefs remaps a slice of attribute refs using the provided mapping
func (m *SeriesMerger) remapRefs(refs []int64, refMap map[int64]int64) []int64 {
	// if we are the first report, we don't have a refMap yet
	if refMap == nil {
		return refs
	}

	result := make([]int64, len(refs))
	for i, ref := range refs {
		if newRef, ok := refMap[ref]; ok {
			result[i] = newRef
		} else {
			panic(fmt.Sprintf("attribute ref %d not found in attribute table", ref))
		}
	}
	return result
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
		return compareAttributeRefs(result[i].AttributeRefs, result[j].AttributeRefs) < 0
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
		s.Points = m.mergePoints(s.Points)
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

		// Expand series attribute_refs back to labels
		seriesLabels := make([]*typesv1.LabelPair, len(s.AttributeRefs))
		labelCount := 0
		for _, ref := range s.AttributeRefs {
			key, value, ok := m.attributeTable.GetKeyValue(ref)
			if ok {
				seriesLabels[labelCount] = &typesv1.LabelPair{
					Name:  key,
					Value: value,
				}
				labelCount++
			}
		}
		seriesLabels = seriesLabels[:labelCount]

		result = append(result, &typesv1.Series{
			Labels: seriesLabels,
			Points: points,
		})
	}

	// Sort series by labels
	sort.Slice(result, func(i, j int) bool {
		return compareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})

	return result
}

// seriesKey creates a unique string key from attribute refs (following heatmap pattern)
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

// compareAttributeRefs compares two slices of attribute refs lexicographically
func compareAttributeRefs(a, b []int64) int {
	l := len(a)
	if len(b) < l {
		l = len(b)
	}

	for i := 0; i < l; i++ {
		if a[i] < b[i] {
			return -1
		} else if a[i] > b[i] {
			return 1
		}
	}

	// If all refs are equal so far, the shorter slice is "less"
	if len(a) < len(b) {
		return -1
	} else if len(a) > len(b) {
		return 1
	}

	return 0
}

// compareLabelPairs compares two slices of label pairs lexicographically.
func compareLabelPairs(a, b []*typesv1.LabelPair) int {
	l := len(a)
	if len(b) < l {
		l = len(b)
	}

	for i := 0; i < l; i++ {
		if a[i].Name < b[i].Name {
			return -1
		} else if a[i].Name > b[i].Name {
			return 1
		}

		if a[i].Value < b[i].Value {
			return -1
		} else if a[i].Value > b[i].Value {
			return 1
		}
	}

	// If all labels are equal so far, the shorter slice is "less"
	if len(a) < len(b) {
		return -1
	} else if len(a) > len(b) {
		return 1
	}

	return 0
}
