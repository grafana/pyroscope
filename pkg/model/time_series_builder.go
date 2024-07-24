package model

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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

func (s *TimeSeriesBuilder) Add(fp model.Fingerprint, lbs Labels, ts int64, value float64) {
	labelsByString, ok := s.labelsByFingerprint[fp]
	if !ok {
		s.labelBuf = lbs.BytesWithLabels(s.labelBuf, s.by...)
		labelsByString = string(s.labelBuf)
		s.labelsByFingerprint[fp] = labelsByString
		if _, ok := s.series[labelsByString]; !ok {
			s.series[labelsByString] = &typesv1.Series{
				Labels: lbs.WithLabels(s.by...),
				Points: []*typesv1.Point{
					{
						Timestamp: ts,
						Value:     value,
					},
				},
			}
			return
		}
	}
	series := s.series[labelsByString]
	series.Points = append(series.Points, &typesv1.Point{
		Timestamp: ts,
		Value:     value,
	})
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
	// we have to sort the points in each series because labels reduction may have changed the order
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result
}
