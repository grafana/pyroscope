package model

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// MaxExemplarCandidatesPerPoint is the maximum number of exemplar candidates
// to collect per point in the Builder.
const MaxExemplarCandidatesPerPoint = 10000

type TimeSeriesBuilder struct {
	labelsByFingerprint map[model.Fingerprint]string
	labelBuf            []byte
	by                  []string

	series seriesByLabels

	exemplarCandidates    map[string]map[int64][]exemplarCandidate
	maxExemplarCandidates int
}

func NewTimeSeriesBuilder(by ...string) *TimeSeriesBuilder {
	var b TimeSeriesBuilder
	b.Init(by...)
	return &b
}

func (s *TimeSeriesBuilder) Init(by ...string) {
	s.labelsByFingerprint = map[model.Fingerprint]string{}
	s.series = make(seriesByLabels)
	s.labelBuf = make([]byte, 0, 1024)
	s.by = by
	s.exemplarCandidates = make(map[string]map[int64][]exemplarCandidate)
}

func (s *TimeSeriesBuilder) Add(fp model.Fingerprint, lbs Labels, ts int64, value float64, annotations schemav1.Annotations, profileID string) {
	labelsByString, ok := s.labelsByFingerprint[fp]
	pAnnotations := make([]*typesv1.ProfileAnnotation, 0, len(annotations.Keys))
	for i := range len(annotations.Keys) {
		pAnnotations = append(pAnnotations, &typesv1.ProfileAnnotation{
			Key:   annotations.Keys[i],
			Value: annotations.Values[i],
		})
	}
	if !ok {
		s.labelBuf = lbs.BytesWithLabels(s.labelBuf, s.by...)
		labelsByString = string(s.labelBuf)
		s.labelsByFingerprint[fp] = labelsByString
		if _, ok := s.series[labelsByString]; !ok {
			s.series[labelsByString] = &typesv1.Series{
				Labels: lbs.WithLabels(s.by...),
				Points: []*typesv1.Point{
					{
						Timestamp:   ts,
						Value:       value,
						Annotations: pAnnotations,
					},
				},
			}
			s.trackExemplar(labelsByString, ts, profileID, value, fp)
			return
		}
	}
	series := s.series[labelsByString]
	series.Points = append(series.Points, &typesv1.Point{
		Timestamp:   ts,
		Value:       value,
		Annotations: pAnnotations,
	})
	s.trackExemplar(labelsByString, ts, profileID, value, fp)
}

// trackExemplar tracks a profile as a potential exemplar for a specific Point.
// Keeps up to maxExemplarCandidates (or MaxExemplarCandidatesPerPoint if not set).
func (s *TimeSeriesBuilder) trackExemplar(seriesKey string, ts int64, profileID string, value float64, fp model.Fingerprint) {
	if profileID == "" {
		return
	}

	if s.exemplarCandidates[seriesKey] == nil {
		s.exemplarCandidates[seriesKey] = make(map[int64][]exemplarCandidate)
	}

	candidates := s.exemplarCandidates[seriesKey][ts]

	limit := s.maxExemplarCandidates
	if limit == 0 {
		limit = MaxExemplarCandidatesPerPoint
	}

	if len(candidates) >= limit {
		return
	}

	candidate := exemplarCandidate{
		profileID:   profileID,
		value:       uint64(value),
		fingerprint: fp,
	}

	s.exemplarCandidates[seriesKey][ts] = append(candidates, candidate)
}

func (s *TimeSeriesBuilder) Build() []*typesv1.Series {
	return s.series.normalize()
}

// BuildWithFullLabels builds the series and enriches exemplars with labels.
func (s *TimeSeriesBuilder) BuildWithFullLabels(fullLabelsByFingerprint map[model.Fingerprint]Labels) []*typesv1.Series {
	series := s.series.normalize()
	s.attachExemplars(series, fullLabelsByFingerprint)
	return series
}

// GetExemplarFingerprints returns all unique fingerprints that have exemplars.
func (s *TimeSeriesBuilder) GetExemplarFingerprints() []model.Fingerprint {
	fingerprintSet := make(map[model.Fingerprint]struct{})
	for _, exemplarsByTimestamp := range s.exemplarCandidates {
		for _, candidates := range exemplarsByTimestamp {
			for _, candidate := range candidates {
				fingerprintSet[candidate.fingerprint] = struct{}{}
			}
		}
	}

	fingerprints := make([]model.Fingerprint, 0, len(fingerprintSet))
	for fp := range fingerprintSet {
		fingerprints = append(fingerprints, fp)
	}
	return fingerprints
}

// attachExemplars adds exemplars to Points based on tracked candidates.
// If fullLabelsByFingerprint is provided, exemplars are enriched with full labels.
func (s *TimeSeriesBuilder) attachExemplars(series []*typesv1.Series, fullLabelsByFingerprint map[model.Fingerprint]Labels) {
	seriesMap := make(map[string]*typesv1.Series)
	for _, ser := range series {
		seriesMap[string(Labels(ser.Labels).BytesWithLabels(nil, s.by...))] = ser
	}

	for seriesKey, exemplarsByTimestamp := range s.exemplarCandidates {
		ser, found := seriesMap[seriesKey]
		if !found {
			continue
		}

		// Attach exemplars to ALL points with matching timestamps
		for _, point := range ser.Points {
			candidates := exemplarsByTimestamp[point.Timestamp]
			if len(candidates) == 0 {
				continue
			}

			point.Exemplars = make([]*typesv1.Exemplar, 0, len(candidates))
			for _, candidate := range candidates {
				var labels Labels
				if fullLabelsByFingerprint != nil {
					labels = fullLabelsByFingerprint[candidate.fingerprint]
				}
				point.Exemplars = append(point.Exemplars, &typesv1.Exemplar{
					Timestamp: point.Timestamp,
					ProfileId: candidate.profileID,
					SpanId:    "",
					Value:     candidate.value,
					Labels:    labels,
				})
			}
		}
	}
}

// FilterNonGroupedLabels returns only labels that are NOT in the groupBy list.
func FilterNonGroupedLabels(fullLabels Labels, groupBy []string) []*typesv1.LabelPair {
	if len(groupBy) == 0 {
		return fullLabels
	}

	grouped := make(map[string]struct{}, len(groupBy))
	for _, name := range groupBy {
		grouped[name] = struct{}{}
	}

	result := make([]*typesv1.LabelPair, 0, len(fullLabels))
	for _, label := range fullLabels {
		if _, isGrouped := grouped[label.Name]; !isGrouped {
			result = append(result, label)
		}
	}
	return result
}

type seriesByLabels map[string]*typesv1.Series

func (m seriesByLabels) normalize() []*typesv1.Series {
	result := lo.Values(m)
	sort.Slice(result, func(i, j int) bool {
		return CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})
	// we have to sort the points in each series because labels reduction may have changed the order
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result
}
