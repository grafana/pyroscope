package timeseriescompact

import (
	"slices"
	"sort"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model/timeseries"
)

// Aggregator aggregates compact points within a time step.
type Aggregator struct {
	ts             int64
	value          float64
	annotationRefs []int64
	exemplars      []*queryv1.Exemplar
	exemplarByID   map[string]exemplarEntry
	hasData        bool
}

type exemplarEntry struct {
	idx     int
	refSets [][]int64
}

// Add adds a point to the aggregator.
func (a *Aggregator) Add(ts int64, v *CompactValue) {
	a.ts = ts
	a.value += v.Value
	a.annotationRefs = append(a.annotationRefs, v.AnnotationRefs...)
	if len(v.Exemplars) > 0 {
		a.addExemplars(v.Exemplars)
	}
	a.hasData = true
}

func (a *Aggregator) addExemplars(exemplars []*queryv1.Exemplar) {
	if a.exemplarByID == nil {
		a.exemplarByID = make(map[string]exemplarEntry, len(exemplars))
	}
	for _, ex := range exemplars {
		entry, found := a.exemplarByID[ex.ProfileId]
		if !found {
			a.exemplarByID[ex.ProfileId] = exemplarEntry{
				idx: len(a.exemplars),
			}
			a.exemplars = append(a.exemplars, ex)
			continue
		}
		existing := a.exemplars[entry.idx]
		if len(entry.refSets) == 0 {
			entry.refSets = [][]int64{existing.AttributeRefs, ex.AttributeRefs}
		} else {
			entry.refSets = append(entry.refSets, ex.AttributeRefs)
		}
		if ex.Value > existing.Value {
			a.exemplars[entry.idx] = ex
		}
		a.exemplarByID[ex.ProfileId] = entry
	}
}

// GetAndReset returns the aggregated point and resets the aggregator.
func (a *Aggregator) GetAndReset() *queryv1.Point {
	pt := &queryv1.Point{
		Timestamp: a.ts,
		Value:     a.value,
	}
	if len(a.annotationRefs) > 0 {
		pt.AnnotationRefs = DedupeRefs(a.annotationRefs)
	}
	if len(a.exemplars) > 0 {
		top := SelectTopExemplars(a.exemplars, timeseries.DefaultMaxExemplarsPerPoint)
		for i, ex := range top {
			if entry := a.exemplarByID[ex.ProfileId]; len(entry.refSets) > 1 {
				top[i] = &queryv1.Exemplar{
					Timestamp:     ex.Timestamp,
					ProfileId:     ex.ProfileId,
					SpanId:        ex.SpanId,
					Value:         ex.Value,
					AttributeRefs: IntersectRefs(entry.refSets),
				}
			}
		}
		pt.Exemplars = top
	}

	// Reset
	a.ts = 0
	a.value = 0
	a.annotationRefs = nil
	clear(a.exemplars)
	a.exemplars = a.exemplars[:0]
	clear(a.exemplarByID)
	a.hasData = false

	return pt
}

// IsEmpty returns true if no data has been added.
func (a *Aggregator) IsEmpty() bool { return !a.hasData }

// Timestamp returns the current timestamp.
func (a *Aggregator) Timestamp() int64 { return a.ts }

// DedupeRefs removes duplicate refs and returns sorted unique refs.
func DedupeRefs(refs []int64) []int64 {
	if len(refs) <= 1 {
		return refs
	}
	slices.Sort(refs)
	j := 0
	for i := 1; i < len(refs); i++ {
		if refs[j] != refs[i] {
			j++
			refs[j] = refs[i]
		}
	}
	return refs[:j+1]
}

// SelectTopExemplars selects the top N exemplars by value.
func SelectTopExemplars(exemplars []*queryv1.Exemplar, n int) []*queryv1.Exemplar {
	if n <= 0 || len(exemplars) == 0 {
		return nil
	}
	if len(exemplars) <= n {
		res := make([]*queryv1.Exemplar, len(exemplars))
		copy(res, exemplars)
		if len(res) > 1 {
			sort.Slice(res, func(i, j int) bool {
				return exemplarBetter(res[i], res[j])
			})
		}
		return res
	}
	if n == 1 {
		best := exemplars[0]
		for _, ex := range exemplars[1:] {
			if exemplarBetter(ex, best) {
				best = ex
			}
		}
		return []*queryv1.Exemplar{best}
	}
	res := make([]*queryv1.Exemplar, len(exemplars))
	copy(res, exemplars)
	sort.Slice(res, func(i, j int) bool {
		return exemplarBetter(res[i], res[j])
	})
	return res[:n]
}

func exemplarBetter(a, b *queryv1.Exemplar) bool {
	if a.Value != b.Value {
		return a.Value > b.Value
	}
	return a.ProfileId < b.ProfileId
}

// MergeExemplars combines two exemplar lists.
// For exemplars with the same profileID, it keeps the highest value and intersects attribute refs.
func MergeExemplars(a, b []*queryv1.Exemplar) []*queryv1.Exemplar {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	type exemplarGroup struct {
		exemplar *queryv1.Exemplar
		refSets  [][]int64
	}
	byProfileID := make(map[string]*exemplarGroup, len(a)+len(b))

	for _, ex := range a {
		byProfileID[ex.ProfileId] = &exemplarGroup{
			exemplar: ex,
			refSets:  [][]int64{ex.AttributeRefs},
		}
	}

	for _, ex := range b {
		existing, found := byProfileID[ex.ProfileId]
		if !found {
			byProfileID[ex.ProfileId] = &exemplarGroup{
				exemplar: ex,
				refSets:  [][]int64{ex.AttributeRefs},
			}
		} else {
			if ex.Value > existing.exemplar.Value {
				existing.exemplar = ex
			}
			existing.refSets = append(existing.refSets, ex.AttributeRefs)
		}
	}

	result := make([]*queryv1.Exemplar, 0, len(byProfileID))
	for _, group := range byProfileID {
		ex := group.exemplar
		if len(group.refSets) > 1 {
			ex.AttributeRefs = IntersectRefs(group.refSets)
		}
		result = append(result, ex)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].ProfileId < result[j].ProfileId })
	return result
}

// IntersectRefs returns the intersection of multiple ref slices.
func IntersectRefs(refSets [][]int64) []int64 {
	if len(refSets) == 0 {
		return nil
	}
	if len(refSets) == 1 {
		return refSets[0]
	}

	set := make(map[int64]struct{}, len(refSets[0]))
	for _, ref := range refSets[0] {
		set[ref] = struct{}{}
	}

	// Intersect with remaining
	for i := 1; i < len(refSets); i++ {
		newSet := make(map[int64]struct{})
		for _, ref := range refSets[i] {
			if _, ok := set[ref]; ok {
				newSet[ref] = struct{}{}
			}
		}
		set = newSet
	}

	if len(set) == 0 {
		return nil
	}

	result := make([]int64, 0, len(set))
	for ref := range set {
		result = append(result, ref)
	}
	slices.Sort(result)
	return result
}
