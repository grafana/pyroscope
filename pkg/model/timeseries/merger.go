package timeseries

import (
	"slices"
	"sort"
	"strings"
	"sync"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func MergeSeries(aggregation *typesv1.TimeSeriesAggregationType, series ...[]*typesv1.Series) []*typesv1.Series {
	var m *Merger
	if aggregation == nil || *aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM {
		m = NewMerger(true)
	} else {
		m = NewMerger(false)
	}
	for _, s := range series {
		m.MergeTimeSeries(s)
	}
	return m.TimeSeries()
}

type Merger struct {
	mu     sync.Mutex
	series map[uint64]*typesv1.Series
	sum    bool
}

// NewMerger creates a new series merger. If sum is set, samples
// with matching timestamps are summed, otherwise duplicates are retained.
func NewMerger(sum bool) *Merger {
	return &Merger{
		series: make(map[uint64]*typesv1.Series),
		sum:    sum,
	}
}

func (m *Merger) MergeTimeSeries(s []*typesv1.Series) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range s {
		h := phlaremodel.Labels(x.Labels).Hash()
		d, ok := m.series[h]
		if !ok {
			m.series[h] = x
			continue
		}
		d.Points = append(d.Points, x.Points...)
	}
}

func (m *Merger) IsEmpty() bool {
	return len(m.series) == 0
}

func (m *Merger) TimeSeries() []*typesv1.Series {
	r := m.mergeTimeSeries()
	sort.Slice(r, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(r[i].Labels, r[j].Labels) < 0
	})
	return r
}

func (m *Merger) mergeTimeSeries() []*typesv1.Series {
	if len(m.series) == 0 {
		return nil
	}
	r := make([]*typesv1.Series, len(m.series))
	var i int
	for _, s := range m.series {
		s.Points = s.Points[:m.mergePoints(s.Points)]
		r[i] = s
		i++
	}
	return r
}

func (m *Merger) Top(n int) []*typesv1.Series {
	return TopSeries(m.mergeTimeSeries(), n)
}

func (m *Merger) mergePoints(points []*typesv1.Point) int {
	l := len(points)
	if l < 2 {
		return l
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp < points[j].Timestamp
	})
	var j int
	for i := 1; i < l; i++ {
		if points[j].Timestamp != points[i].Timestamp || !m.sum {
			j++
			points[j] = points[i]
			continue
		}
		if m.sum {
			points[j].Value += points[i].Value
			points[j].Annotations = mergeAnnotations(points[j].Annotations, points[i].Annotations)
			points[j].Exemplars = mergeExemplars(points[j].Exemplars, points[i].Exemplars)
		}
	}
	return j + 1
}

func compareAnnotations(a, b *typesv1.ProfileAnnotation) int {
	if r := strings.Compare(a.Key, b.Key); r != 0 {
		return r
	}
	return strings.Compare(a.Value, b.Value)
}

func mergeAnnotations(a, b []*typesv1.ProfileAnnotation) []*typesv1.ProfileAnnotation {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	// Merge into a single slice
	merged := append(a, b...)

	// Sort by key and value
	slices.SortFunc(merged, compareAnnotations)

	// Remove duplicates in-place
	j := 0
	for i := 1; i < len(merged); i++ {
		// Only keep if different from the current unique element
		if merged[j].Key != merged[i].Key || merged[j].Value != merged[i].Value {
			j++
			merged[j] = merged[i]
		}
	}

	// Return the slice with only unique elements
	return merged[:j+1]
}

// mergeExemplars combines two exemplar lists.
// For exemplars with the same profileID, it keeps the highest value and intersects labels.
func mergeExemplars(a, b []*typesv1.Exemplar) []*typesv1.Exemplar {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	type exemplarGroup struct {
		exemplar  *typesv1.Exemplar
		labelSets []phlaremodel.Labels
	}
	byProfileID := make(map[string]*exemplarGroup)

	for _, ex := range a {
		byProfileID[ex.ProfileId] = &exemplarGroup{
			exemplar:  ex,
			labelSets: []phlaremodel.Labels{ex.Labels},
		}
	}

	for _, ex := range b {
		existing, found := byProfileID[ex.ProfileId]
		if !found {
			byProfileID[ex.ProfileId] = &exemplarGroup{
				exemplar:  ex,
				labelSets: []phlaremodel.Labels{phlaremodel.Labels(ex.Labels)},
			}
		} else {
			if ex.Value > existing.exemplar.Value {
				existing.exemplar = ex
			}
			existing.labelSets = append(existing.labelSets, phlaremodel.Labels(ex.Labels))
		}
	}

	result := make([]*typesv1.Exemplar, 0, len(byProfileID))
	for _, group := range byProfileID {
		ex := group.exemplar
		if len(group.labelSets) > 1 {
			intersected := phlaremodel.IntersectAll(group.labelSets)
			if intersected == nil && ex.Labels != nil {
				ex.Labels = []*typesv1.LabelPair{}
			} else {
				ex.Labels = intersected
			}
		}
		result = append(result, ex)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ProfileId < result[j].ProfileId
	})

	return result
}
