// Package timeseries provides types for building and aggregating time series data.
//
// NOTE: This is the old time series implementation using string labels.
// Currently used for all time series queries except exemplar retrieval.
// Over time, we want to migrate to pkg/model/timeseriescompact which uses
// attribute table interning for better performance, and remove this package.
package timeseries

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type Builder struct {
	labelBuf []byte
	by       []string

	series seriesByLabels

	exemplarBuilders map[string]*exemplarBuilder
}

func NewBuilder(by ...string) *Builder {
	var b Builder
	b.Init(by...)
	return &b
}

func (s *Builder) Init(by ...string) {
	s.series = make(seriesByLabels)
	s.labelBuf = make([]byte, 0, 1024)
	s.by = by
	s.exemplarBuilders = make(map[string]*exemplarBuilder)
}

// Add adds a data point with full labels.
// The series is grouped by the 'by' labels, but exemplars retain full labels.
func (s *Builder) Add(fp model.Fingerprint, lbs phlaremodel.Labels, ts int64, value float64, annotations schemav1.Annotations, profileID string) {
	s.labelBuf = lbs.BytesWithLabels(s.labelBuf, s.by...)
	seriesKey := string(s.labelBuf)

	pAnnotations := make([]*typesv1.ProfileAnnotation, 0, len(annotations.Keys))
	for i := range len(annotations.Keys) {
		pAnnotations = append(pAnnotations, &typesv1.ProfileAnnotation{
			Key:   annotations.Keys[i],
			Value: annotations.Values[i],
		})
	}

	series, exists := s.series[seriesKey]
	if !exists {
		series = &typesv1.Series{
			Labels: lbs.WithLabels(s.by...),
			Points: make([]*typesv1.Point, 0),
		}
		s.series[seriesKey] = series
	}

	series.Points = append(series.Points, &typesv1.Point{
		Timestamp:   ts,
		Value:       value,
		Annotations: pAnnotations,
	})

	if profileID != "" {
		if s.exemplarBuilders[seriesKey] == nil {
			s.exemplarBuilders[seriesKey] = newExemplarBuilder()
		}
		exemplarLabels := lbs.WithoutLabels(s.by...)
		s.exemplarBuilders[seriesKey].Add(fp, exemplarLabels, ts, profileID, int64(value))
	}
}

// Build returns the time series without exemplars.
func (s *Builder) Build() []*typesv1.Series {
	return s.series.normalize()
}

// BuildWithExemplars returns the time series with exemplars attached.
func (s *Builder) BuildWithExemplars() []*typesv1.Series {
	series := s.series.normalize()
	s.attachExemplars(series)
	return series
}

// ExemplarCount returns the number of raw exemplars added (before deduplication).
func (s *Builder) ExemplarCount() int {
	total := 0
	for _, builder := range s.exemplarBuilders {
		total += builder.Count()
	}
	return total
}

// attachExemplars attaches exemplars from exemplarBuilders to the corresponding points.
func (s *Builder) attachExemplars(series []*typesv1.Series) {
	// Create a map from seriesKey to series for fast lookup
	seriesMap := make(map[string]*typesv1.Series)
	for _, ser := range series {
		seriesKey := string(phlaremodel.Labels(ser.Labels).BytesWithLabels(nil, s.by...))
		seriesMap[seriesKey] = ser
	}

	for seriesKey, exemplarBuilder := range s.exemplarBuilders {
		ser, found := seriesMap[seriesKey]
		if !found {
			continue
		}

		exemplars := exemplarBuilder.Build()
		if len(exemplars) == 0 {
			continue
		}

		// Attach exemplars to points with matching timestamps
		// Both exemplars and points are sorted by timestamp
		exIdx := 0
		for _, point := range ser.Points {
			// Skip exemplars with timestamp < point timestamp
			for exIdx < len(exemplars) && exemplars[exIdx].Timestamp < point.Timestamp {
				exIdx++
			}

			// Collect all exemplars with timestamp == point timestamp
			var pointExemplars []*typesv1.Exemplar
			for i := exIdx; i < len(exemplars) && exemplars[i].Timestamp == point.Timestamp; i++ {
				pointExemplars = append(pointExemplars, exemplars[i])
			}

			if len(pointExemplars) > 0 {
				point.Exemplars = pointExemplars
			}
		}
	}
}

type seriesByLabels map[string]*typesv1.Series

func (m seriesByLabels) normalize() []*typesv1.Series {
	result := lo.Values(m)
	sort.Slice(result, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result
}
