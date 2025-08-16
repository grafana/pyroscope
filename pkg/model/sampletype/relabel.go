package sampletype

import (
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/slices"
	"github.com/grafana/pyroscope/pkg/validation"

	"github.com/prometheus/prometheus/model/relabel"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlarerelabel "github.com/grafana/pyroscope/pkg/model/relabel"
)

func Relabel(p validation.ValidatedProfile, rules []*relabel.Config, labels []*typesv1.LabelPair) {
	if len(rules) == 0 {
		return
	}
	keeps := make([]bool, len(p.SampleType))
	for i, st := range p.SampleType {
		lb := phlaremodel.NewLabelsBuilder(labels)
		lb.Set("__type__", p.StringTable[st.Type])
		lb.Set("__unit__", p.StringTable[st.Unit])
		_, keep := phlarerelabel.Process(lb.Labels(), rules...)
		keeps[i] = keep
	}
	p.SampleType = slices.RemoveInPlace(p.SampleType, func(_ *googlev1.ValueType, idx int) bool {
		return !keeps[idx]
	})
	for _, sample := range p.Sample {
		sample.Value = slices.RemoveInPlace(sample.Value, func(_ int64, idx int) bool {
			return !keeps[idx]
		})
	}
}
