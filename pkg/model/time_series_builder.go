package model

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type TimeSeriesBuilder struct {
	labelBuf []byte
	by       []string

	series seriesByLabels

	exemplarBuilders map[string]*ExemplarBuilder
	attributeTable   *attributetable.Table
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
	s.attributeTable = attributetable.NewTable()
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
			s.exemplarBuilders[seriesKey] = NewExemplarBuilder()
		}
		exemplarLabels := lbs.WithoutLabels(s.by...)
		s.exemplarBuilders[seriesKey].Add(fp, exemplarLabels, ts, profileID, uint64(value))
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

// BuildWithAttributeTable returns the time series with exemplars using AttributeTable optimization.
func (s *TimeSeriesBuilder) BuildWithAttributeTable() []*queryv1.Series {
	series := s.series.normalize()
	return s.attachExemplarsWithAttributeTable(series)
}

// ExemplarCount returns the number of raw exemplars added (before deduplication).
func (s *TimeSeriesBuilder) ExemplarCount() int {
	total := 0
	for _, builder := range s.exemplarBuilders {
		total += builder.Count()
	}
	return total
}

// AttributeTable returns the attribute table built during BuildWithExemplars().
func (s *TimeSeriesBuilder) AttributeTable() *attributetable.Table {
	return s.attributeTable
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

		exemplarBuilder.Build()

		var exemplars []*typesv1.Exemplar
		exemplarBuilder.ForEach(func(labels Labels, ts int64, profileID string, value uint64) {
			ex := &typesv1.Exemplar{
				Timestamp: ts,
				ProfileId: profileID,
				Value:     value,
				Labels:    []*typesv1.LabelPair(labels),
			}
			exemplars = append(exemplars, ex)
		})

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

// attachExemplarsWithAttributeTable attaches exemplars using AttributeTable optimization.
func (s *TimeSeriesBuilder) attachExemplarsWithAttributeTable(series []*typesv1.Series) []*queryv1.Series {
	result := make([]*queryv1.Series, len(series))
	seriesMap := make(map[string]*queryv1.Series)

	for i, ts := range series {
		points := make([]*queryv1.Point, len(ts.Points))
		for j, p := range ts.Points {
			points[j] = &queryv1.Point{
				Value:       p.Value,
				Timestamp:   p.Timestamp,
				Annotations: p.Annotations,
			}
		}
		querySeries := &queryv1.Series{
			Labels: ts.Labels,
			Points: points,
		}
		result[i] = querySeries

		seriesKey := string(Labels(ts.Labels).BytesWithLabels(nil, s.by...))
		seriesMap[seriesKey] = querySeries
	}

	for seriesKey, exemplarBuilder := range s.exemplarBuilders {
		querySeries, found := seriesMap[seriesKey]
		if !found {
			continue
		}

		exemplarBuilder.Build()

		var exemplars []*queryv1.Exemplar
		exemplarBuilder.ForEach(func(labels Labels, ts int64, profileID string, value uint64) {
			ex := &queryv1.Exemplar{
				Timestamp:     ts,
				ProfileId:     profileID,
				Value:         value,
				AttributeRefs: nil,
			}
			ex.AttributeRefs = s.attributeTable.Refs(labels, ex.AttributeRefs)
			exemplars = append(exemplars, ex)
		})

		if len(exemplars) == 0 {
			continue
		}

		exIdx := 0
		for _, point := range querySeries.Points {
			// Skip exemplars with timestamp < point timestamp
			for exIdx < len(exemplars) && exemplars[exIdx].Timestamp < point.Timestamp {
				exIdx++
			}

			// Collect all exemplars with timestamp == point timestamp
			for i := exIdx; i < len(exemplars) && exemplars[i].Timestamp == point.Timestamp; i++ {
				point.Exemplars = append(point.Exemplars, exemplars[i])
			}
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
