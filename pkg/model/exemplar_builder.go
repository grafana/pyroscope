package model

import (
	"sort"

	"github.com/prometheus/common/model"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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

// Build returns the final exemplars, sorted and deduplicated.
// Exemplars with the same (profileID, timestamp) are merged by intersecting their labels.
func (eb *ExemplarBuilder) Build() []*typesv1.Exemplar {
	if len(eb.exemplars) == 0 {
		return nil
	}

	sort.Slice(eb.exemplars, func(i, j int) bool {
		if eb.exemplars[i].timestamp != eb.exemplars[j].timestamp {
			return eb.exemplars[i].timestamp < eb.exemplars[j].timestamp
		}
		return eb.exemplars[i].profileID < eb.exemplars[j].profileID
	})

	return eb.deduplicateAndIntersect()
}

// deduplicateAndIntersect merges exemplars with the same profileID, timestamp by intersecting their label sets.
// When multiple exemplars exist for the same (profileID, timestamp), we sum their values.
func (eb *ExemplarBuilder) deduplicateAndIntersect() []*typesv1.Exemplar {
	result := make([]*typesv1.Exemplar, 0, len(eb.exemplars))

	i := 0
	for i < len(eb.exemplars) {
		curr := eb.exemplars[i]

		labelSetsToIntersect := []Labels{eb.labelSets[curr.labelSetRef]}
		sumValue := curr.value
		j := i + 1
		for j < len(eb.exemplars) &&
			eb.exemplars[j].profileID == curr.profileID &&
			eb.exemplars[j].timestamp == curr.timestamp {
			labelSetsToIntersect = append(labelSetsToIntersect, eb.labelSets[eb.exemplars[j].labelSetRef])
			sumValue += eb.exemplars[j].value
			j++
		}

		finalLabels := IntersectAll(labelSetsToIntersect)

		result = append(result, &typesv1.Exemplar{
			Timestamp: curr.timestamp,
			ProfileId: curr.profileID,
			Value:     sumValue,
			Labels:    finalLabels,
		})

		i = j
	}

	return result
}
