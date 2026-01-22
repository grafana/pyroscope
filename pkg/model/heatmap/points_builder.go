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
	value       int64
}

// newPointsBuilder creates a new pointsBuilder.
func newPointsBuilder() *pointsBuilder {
	return &pointsBuilder{
		labelSetIndex: make(map[uint64]int),
		labelSets:     make([]model.Labels, 0),
		exemplars:     make([]exemplar, 0),
	}
}

// Add adds an exemplar with its full labels.
func (pb *pointsBuilder) add(fp prommodel.Fingerprint, labels model.Labels, ts int64, profileID string, spanID uint64, value int64) {
	if profileID == "" && spanID == 0 {
		return
	}

	e := exemplar{
		timestamp: ts,
		profileID: profileID,
		spanID:    spanID,
		value:     value,
	}

	labelSetIdx, labelSetExists := pb.labelSetIndex[uint64(fp)]

	pos, exemplarExists := slices.BinarySearchFunc(pb.exemplars, e, cmpExemplar)
	if exemplarExists {
		matched := &pb.exemplars[pos]
		// add my value
		matched.value += e.value

		// check if label set matches
		if labelSetExists && labelSetIdx == matched.labelSetRef {
			return
		}

		// build intersected label set, they are cloned so this is possible to do
		pb.labelSets[matched.labelSetRef] = pb.labelSets[matched.labelSetRef].Intersect(labels)

		// point to new label set
		pb.labelSetIndex[uint64(fp)] = matched.labelSetRef

		return
	}

	if !labelSetExists {
		e.labelSetRef = len(pb.labelSets)
		pb.labelSets = append(pb.labelSets, labels.Clone())
		pb.labelSetIndex[uint64(fp)] = e.labelSetRef
	} else {
		// Use the existing label set for this fingerprint, but intersect with new labels
		// This ensures only common labels across all exemplars with this fingerprint are kept
		e.labelSetRef = labelSetIdx
		pb.labelSets[e.labelSetRef] = pb.labelSets[e.labelSetRef].Intersect(labels)
	}

	pb.exemplars = slices.Insert(pb.exemplars, pos, e)
}

func cmpExemplar(a, b exemplar) int {
	if a.timestamp < b.timestamp {
		return -1
	}
	if a.timestamp > b.timestamp {
		return 1
	}
	if a.spanID < b.spanID {
		return -1
	}
	if a.spanID > b.spanID {
		return 1
	}
	return strings.Compare(a.profileID, b.profileID)
}

func (pb *pointsBuilder) forEach(f func(labels model.Labels, ts int64, profileID string, spanID uint64, value int64)) {
	for _, exemplar := range pb.exemplars {
		f(pb.labelSets[exemplar.labelSetRef], exemplar.timestamp, exemplar.profileID, exemplar.spanID, exemplar.value)
	}
}

// Count returns the number of raw exemplars added.
func (pb *pointsBuilder) count() int {
	return len(pb.exemplars)
}
