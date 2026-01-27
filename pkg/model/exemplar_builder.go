package model

import (
	"sort"

	"github.com/prometheus/common/model"
)

// ExemplarBuilder builds exemplars for a single time series.
type ExemplarBuilder struct {
	labelSetIndex map[uint64]int
	labelSets     []Labels
	exemplars     []exemplar
}

type exemplar struct {
	timestamp   int64
	profileID   string
	labelSetRef int
	value       int64
}

// NewExemplarBuilder creates a new ExemplarBuilder.
func NewExemplarBuilder() *ExemplarBuilder {
	return &ExemplarBuilder{
		labelSetIndex: make(map[uint64]int),
		labelSets:     make([]Labels, 0),
		exemplars:     make([]exemplar, 0),
	}
}

// Add adds an exemplar with its full labels.
func (eb *ExemplarBuilder) Add(fp model.Fingerprint, labels Labels, ts int64, profileID string, value int64) {
	if profileID == "" {
		return
	}

	labelSetIdx, exists := eb.labelSetIndex[uint64(fp)]
	if !exists {
		// Clone labels to avoid external mutations
		eb.labelSets = append(eb.labelSets, labels.Clone())
		labelSetIdx = len(eb.labelSets) - 1
		eb.labelSetIndex[uint64(fp)] = labelSetIdx
	}

	eb.exemplars = append(eb.exemplars, exemplar{
		timestamp:   ts,
		profileID:   profileID,
		labelSetRef: labelSetIdx,
		value:       value,
	})
}

// Count returns the number of raw exemplars added.
func (eb *ExemplarBuilder) Count() int {
	return len(eb.exemplars)
}

// Build sorts and deduplicates exemplars.
func (eb *ExemplarBuilder) Build() {
	if len(eb.exemplars) == 0 {
		return
	}

	// Sort by timestamp, then profileID
	sort.Slice(eb.exemplars, func(i, j int) bool {
		if eb.exemplars[i].timestamp != eb.exemplars[j].timestamp {
			return eb.exemplars[i].timestamp < eb.exemplars[j].timestamp
		}
		return eb.exemplars[i].profileID < eb.exemplars[j].profileID
	})

	eb.deduplicateAndIntersect()
}

// deduplicateAndIntersect merges exemplars with the same (profileID, timestamp).
// Their label sets are intersected and values are summed.
func (eb *ExemplarBuilder) deduplicateAndIntersect() {
	if len(eb.exemplars) == 0 {
		return
	}

	deduplicated := make([]exemplar, 0, len(eb.exemplars))
	i := 0

	for i < len(eb.exemplars) {
		curr := eb.exemplars[i]
		labelSetsToIntersect := []Labels{eb.labelSets[curr.labelSetRef]}
		sumValue := curr.value
		j := i + 1

		// Collect all exemplars with same (profileID, timestamp)
		for j < len(eb.exemplars) &&
			eb.exemplars[j].profileID == curr.profileID &&
			eb.exemplars[j].timestamp == curr.timestamp {
			labelSetsToIntersect = append(labelSetsToIntersect, eb.labelSets[eb.exemplars[j].labelSetRef])
			sumValue += eb.exemplars[j].value
			j++
		}

		finalLabels := intersectAll(labelSetsToIntersect)
		if curr.labelSetRef < len(eb.labelSets) {
			eb.labelSets[curr.labelSetRef] = finalLabels
		} else {
			eb.labelSets = append(eb.labelSets, finalLabels)
			curr.labelSetRef = len(eb.labelSets) - 1
		}

		curr.value = sumValue
		deduplicated = append(deduplicated, curr)

		i = j
	}

	eb.exemplars = deduplicated
}

// intersectAll finds the intersection of multiple label sets.
func intersectAll(labelSets []Labels) Labels {
	if len(labelSets) == 0 {
		return nil
	}
	if len(labelSets) == 1 {
		return labelSets[0]
	}

	result := labelSets[0]
	for i := 1; i < len(labelSets); i++ {
		result = result.Intersect(labelSets[i])
	}

	return result
}

// ForEach iterates over all exemplars, calling the provided function for each one.
func (eb *ExemplarBuilder) ForEach(f func(labels Labels, ts int64, profileID string, value int64)) {
	for _, ex := range eb.exemplars {
		f(eb.labelSets[ex.labelSetRef], ex.timestamp, ex.profileID, ex.value)
	}
}
