package timeseries

import (
	"math"

	"github.com/prometheus/common/model"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type Value struct {
	Ts          int64
	Lbs         []*typesv1.LabelPair
	LabelsHash  uint64
	Value       float64
	Annotations []*typesv1.ProfileAnnotation
	Exemplars   []*typesv1.Exemplar
}

func (p Value) Labels() phlaremodel.Labels { return p.Lbs }
func (p Value) Timestamp() model.Time      { return model.Time(p.Ts) }

type Iterator struct {
	point []*typesv1.Point
	curr  Value
}

func NewSeriesIterator(lbs []*typesv1.LabelPair, points []*typesv1.Point) *Iterator {
	return &Iterator{
		point: points,

		curr: Value{
			Lbs:        lbs,
			LabelsHash: phlaremodel.Labels(lbs).Hash(),
		},
	}
}

func (s *Iterator) Next() bool {
	if len(s.point) == 0 {
		return false
	}
	p := s.point[0]
	s.point = s.point[1:]
	s.curr.Ts = p.Timestamp
	s.curr.Value = p.Value
	s.curr.Annotations = p.Annotations

	s.curr.Exemplars = p.Exemplars
	return true
}

func (s *Iterator) At() Value    { return s.curr }
func (s *Iterator) Err() error   { return nil }
func (s *Iterator) Close() error { return nil }

func NewTimeSeriesMergeIterator(series []*typesv1.Series) iter.Iterator[Value] {
	iters := make([]iter.Iterator[Value], 0, len(series))
	for _, s := range series {
		iters = append(iters, NewSeriesIterator(s.Labels, s.Points))
	}
	return phlaremodel.NewMergeIterator(Value{Ts: math.MaxInt64}, false, iters...)
}
