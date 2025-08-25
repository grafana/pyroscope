package model

import (
	"slices"
	"sort"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type TimeSeriesBuilder struct {
	labelsByFingerprint map[model.Fingerprint]string
	labelBuf            []byte
	by                  []string

	series seriesByLabels
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
}

// mergePoints merges two points with the same timestamp.
func mergePoints(a, b *typesv1.Point) {
	if a.Timestamp != b.Timestamp {
		panic("timestamps do not match")
	}

	a.Value += b.Value
	a.Annotations = append(a.Annotations, b.Annotations...)

	// add the series fingerprints into an ordered slice
	for _, fp := range b.SeriesFingerprints {
		idx, found := slices.BinarySearchFunc(a.SeriesFingerprints, fp, func(a, b uint64) int {
			return int(a - b)
		})
		if found {
			continue
		}
		a.SeriesFingerprints = slices.Insert(a.SeriesFingerprints, idx, fp)
	}
}

// insertPoint inserts a point into a sorted slice of points.
func insertPoint(points []*typesv1.Point, newPoint *typesv1.Point) []*typesv1.Point {
	idx, found := slices.BinarySearchFunc(points, newPoint, PointsOrderTimestampThenProfileID)
	if found {
		mergePoints(points[idx], newPoint)
		return points
	}
	return slices.Insert(points, idx, newPoint)
}

func (s *TimeSeriesBuilder) AddWithProfileID(fp model.Fingerprint, lbs Labels, ts int64, value float64, profileID []byte, annotations schemav1.Annotations) {
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
						Timestamp:          ts,
						Value:              value,
						Annotations:        pAnnotations,
						ProfileId:          string(profileID),
						SeriesFingerprints: []uint64{uint64(fp)},
					},
				},
			}
			return
		}
	}
	series := s.series[labelsByString]
	series.Points = insertPoint(series.Points, &typesv1.Point{
		Timestamp:          ts,
		Value:              value,
		Annotations:        pAnnotations,
		ProfileId:          string(profileID),
		SeriesFingerprints: []uint64{uint64(fp)},
	})
}

func (s *TimeSeriesBuilder) Add(fp model.Fingerprint, lbs Labels, ts int64, value float64, annotations schemav1.Annotations) {
	s.AddWithProfileID(fp, lbs, ts, value, nil, annotations)
}

func (s *TimeSeriesBuilder) Build() []*typesv1.Series {
	return s.series.normalize()
}

type seriesByLabels map[string]*typesv1.Series

func (m seriesByLabels) normalize() []*typesv1.Series {
	result := lo.Values(m)
	sort.Slice(result, func(i, j int) bool {
		return CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})
	return result
}
