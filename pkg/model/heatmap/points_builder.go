package heatmap

import (
	"slices"
	"strings"

	prommodel "github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/model"
)

// pointsBuilder builds exemplars for a single time series.
type pointsBuilder struct {
	labelSetIndex map[uint64]int
	labelSets     []model.Labels
	exemplars     []exemplar
}

type exemplar struct {
	timestamp   int64
	profileID   string
	spanID      uint64
	labelSetRef int
	value       uint64
}

// newPointsBuilder creates a new ExemplarBuilder.
func newPointsBuilder() *pointsBuilder {
	return &pointsBuilder{
		labelSetIndex: make(map[uint64]int),
		labelSets:     make([]model.Labels, 0),
		exemplars:     make([]exemplar, 0),
	}
}

// Add adds an exemplar with its full labels.
func (eb *pointsBuilder) add(fp prommodel.Fingerprint, labels model.Labels, ts int64, profileID string, spanID uint64, value uint64) {
	if profileID == "" && spanID == 0 {
		return
	}

	e := exemplar{
		timestamp: ts,
		profileID: profileID,
		spanID:    spanID,
		value:     value,
	}

	labelSetIdx, labelSetExists := eb.labelSetIndex[uint64(fp)]

	pos, exemplarExists := slices.BinarySearchFunc(eb.exemplars, e, cmpExemplar)
	if exemplarExists {
		matched := &eb.exemplars[pos]
		// add my value
		matched.value += e.value

		// check if label set matches
		if labelSetExists && labelSetIdx == matched.labelSetRef {
			return
		}

		// build intersected label set, they are cloned so this is possible to do
		eb.labelSets[matched.labelSetRef] = eb.labelSets[matched.labelSetRef].Intersect(labels)

		// point to new label set
		eb.labelSetIndex[uint64(fp)] = matched.labelSetRef

		return
	}

	if !labelSetExists {
		e.labelSetRef = len(eb.labelSets)
		eb.labelSets = append(eb.labelSets, labels.Clone())
		eb.labelSetIndex[uint64(fp)] = e.labelSetRef
	} else {
		// Use the existing label set for this fingerprint, but intersect with new labels
		// This ensures only common labels across all exemplars with this fingerprint are kept
		e.labelSetRef = labelSetIdx
		eb.labelSets[e.labelSetRef] = eb.labelSets[e.labelSetRef].Intersect(labels)
	}

	eb.exemplars = slices.Insert(eb.exemplars, pos, e)
}

func cmpExemplar(a, b exemplar) int {
	if r := a.timestamp - b.timestamp; r != 0 {
		return int(r)
	}
	if r := a.spanID - b.spanID; r != 0 {
		return int(r)
	}

	return strings.Compare(a.profileID, b.profileID)
}

func (eb *pointsBuilder) forEach(f func(labels model.Labels, ts int64, profileID string, spanID uint64, value uint64)) {
	for _, exemplar := range eb.exemplars {
		f(eb.labelSets[exemplar.labelSetRef], exemplar.timestamp, exemplar.profileID, exemplar.spanID, exemplar.value)
	}
}

// Count returns the number of raw exemplars added.
func (eb *pointsBuilder) count() int {
	return len(eb.exemplars)
}
