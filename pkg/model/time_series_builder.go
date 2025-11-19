package model

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type TimeSeriesBuilder struct {
	labelBuf []byte
	by       []string

	series seriesByLabels

	exemplarBuilders map[string]*ExemplarBuilder
}

func NewTimeSeriesBuilder(by ...string) *TimeSeriesBuilder {
	var b TimeSeriesBuilder
	b.Init(by...)
	return &b
}

func (s *TimeSeriesBuilder) Init(by ...string) {
	s.series = make(seriesByLabels)
	s.labelBuf = make([]byte, 0, 1024)
	s.by = by
	s.exemplarBuilders = make(map[string]*ExemplarBuilder)
}

// Add adds a data point with full labels.
// The series is grouped by the 'by' labels, but exemplars retain full labels.
func (s *TimeSeriesBuilder) Add(fp model.Fingerprint, lbs Labels, ts int64, value float64, annotations schemav1.Annotations, profileID string) {
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
		s.series[seriesKey] = &typesv1.Series{
			Labels: lbs.WithLabels(s.by...),
			Points: []*typesv1.Point{
				{
					Timestamp:   ts,
					Value:       value,
					Annotations: pAnnotations,
				},
			},
		}
	} else {
		series.Points = append(series.Points, &typesv1.Point{
			Timestamp:   ts,
			Value:       value,
			Annotations: pAnnotations,
		})
	}

	if profileID != "" {
		if s.exemplarBuilders[seriesKey] == nil {
			s.exemplarBuilders[seriesKey] = NewExemplarBuilder()
		}
		s.exemplarBuilders[seriesKey].Add(fp, lbs, ts, profileID, uint64(value))
	}
}

// Build returns the time series without exemplars.
func (s *TimeSeriesBuilder) Build() []*typesv1.Series {
	return s.series.normalize()
}

// BuildWithExemplars returns the time series with exemplars attached.
func (s *TimeSeriesBuilder) BuildWithExemplars() []*typesv1.Series {
	series := s.series.normalize()
	s.attachExemplars(series)
	return series
}

// attachExemplars attaches exemplars from ExemplarBuilders to the corresponding points.
func (s *TimeSeriesBuilder) attachExemplars(series []*typesv1.Series) {
	// Create a map from seriesKey to series for fast lookup
	seriesMap := make(map[string]*typesv1.Series)
	for _, ser := range series {
		seriesKey := string(Labels(ser.Labels).BytesWithLabels(nil, s.by...))
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

		exemplarsByTimestamp := make(map[int64][]*typesv1.Exemplar)
		for _, ex := range exemplars {
			exemplarsByTimestamp[ex.Timestamp] = append(exemplarsByTimestamp[ex.Timestamp], ex)
		}

		for _, point := range ser.Points {
			if exs, found := exemplarsByTimestamp[point.Timestamp]; found {
				point.Exemplars = exs
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
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result
}
