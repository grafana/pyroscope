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
	value       uint64
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
func (eb *ExemplarBuilder) Add(fp model.Fingerprint, labels Labels, ts int64, profileID string, value uint64) {
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
func (eb *ExemplarBuilder) deduplicateAndIntersect() []*typesv1.Exemplar {
	result := make([]*typesv1.Exemplar, 0, len(eb.exemplars))

	i := 0
	for i < len(eb.exemplars) {
		curr := eb.exemplars[i]

		labelSetsToIntersect := []Labels{eb.labelSets[curr.labelSetRef]}
		j := i + 1
		for j < len(eb.exemplars) &&
			eb.exemplars[j].profileID == curr.profileID &&
			eb.exemplars[j].timestamp == curr.timestamp {
			labelSetsToIntersect = append(labelSetsToIntersect, eb.labelSets[eb.exemplars[j].labelSetRef])
			j++
		}

		finalLabels := IntersectAll(labelSetsToIntersect)

		result = append(result, &typesv1.Exemplar{
			Timestamp: curr.timestamp,
			ProfileId: curr.profileID,
			Value:     curr.value,
			Labels:    finalLabels,
		})

		i = j
	}

	return result
}
