package sampletype

import (
	"fmt"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/slices"
	"github.com/grafana/pyroscope/pkg/validation"

	"github.com/prometheus/prometheus/model/relabel"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlarerelabel "github.com/grafana/pyroscope/pkg/model/relabel"
)

func Relabel(p validation.ValidatedProfile, rules []*relabel.Config, series []*typesv1.LabelPair) {
	if len(rules) == 0 {
		return
	}
	keeps := make([]bool, len(p.SampleType))
	for i, st := range p.SampleType {
		builder := phlaremodel.NewLabelsBuilder(series)
		builder.Set("__type__", p.StringTable[st.Type])
		builder.Set("__unit__", p.StringTable[st.Unit])
		labels := builder.Labels()
		fmt.Println(labels)
		_, keep := phlarerelabel.Process(labels, rules...)
		keeps[i] = keep
	}
	fmt.Println("keeps", keeps)
	p.SampleType = slices.RemoveInPlace(p.SampleType, func(_ *googlev1.ValueType, idx int) bool {
		return !keeps[idx]
	})
	for _, sample := range p.Sample {
		sample.Value = slices.RemoveInPlace(sample.Value, func(_ int64, idx int) bool {
			return !keeps[idx]
		})
	}
}
