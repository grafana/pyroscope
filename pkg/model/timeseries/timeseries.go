// Package timeseries provides types for building and aggregating time series data.
//
// NOTE: This is the old time series implementation using string labels.
// Currently used for all time series queries except exemplar retrieval.
// Over time, we want to migrate to pkg/model/timeseriescompact which uses
// attribute table interning for better performance, and remove this package.
package timeseries

import (
	"cmp"
	"slices"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
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

	pAnnotations := make([]*typesv1.ProfileAnnotation, 0, len(annotations.Keys))
	for i := range len(annotations.Keys) {
		pAnnotations = append(pAnnotations, &typesv1.ProfileAnnotation{
			Key:   annotations.Keys[i],
			Value: annotations.Values[i],
		})
	}

	// Inline string(s.labelBuf) map lookups do not allocate; the key is only materialized on insert.
	series, exists := s.series[string(s.labelBuf)]
	if !exists {
		series = &typesv1.Series{
			Labels: lbs.WithLabels(s.by...),
			Points: make([]*typesv1.Point, 0),
		}
		s.series[string(s.labelBuf)] = series
	}

	series.Points = append(series.Points, &typesv1.Point{
		Timestamp:   ts,
		Value:       value,
		Annotations: pAnnotations,
	})

	if profileID != "" {
		eb := s.exemplarBuilders[string(s.labelBuf)]
		if eb == nil {
			eb = newExemplarBuilder()
			s.exemplarBuilders[string(s.labelBuf)] = eb
		}
		exemplarLabels := lbs.WithoutLabels(s.by...)
		eb.Add(fp, exemplarLabels, ts, profileID, int64(value))
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
	slices.SortFunc(result, func(a, b *typesv1.Series) int {
		return phlaremodel.CompareLabelPairs(a.Labels, b.Labels)
	})
	for _, s := range result {
		slices.SortFunc(s.Points, func(a, b *typesv1.Point) int {
			return cmp.Compare(a.Timestamp, b.Timestamp)
		})
	}
	return result
}
