package labels

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func TestLabelsForProfiles(t *testing.T) {
	for _, tt := range []struct {
		name     string
		in       phlaremodel.Labels
		expected phlaremodel.Labels
	}{
		{
			"default",
			phlaremodel.Labels{{Name: model.MetricNameLabel, Value: "cpu"}},
			phlaremodel.Labels{
				{Name: phlaremodel.LabelNameProfileType, Value: "cpu:type:unit:type:unit"},
				{Name: model.MetricNameLabel, Value: "cpu"},
				{Name: phlaremodel.LabelNamePeriodType, Value: "type"},
				{Name: phlaremodel.LabelNamePeriodUnit, Value: "unit"},
				{Name: phlaremodel.LabelNameType, Value: "type"},
				{Name: phlaremodel.LabelNameUnit, Value: "unit"},
			},
		},
		{
			"with service_name",
			phlaremodel.Labels{
				{Name: model.MetricNameLabel, Value: "cpu"},
				{Name: phlaremodel.LabelNameServiceName, Value: "service_name"},
			},
			phlaremodel.Labels{
				{Name: phlaremodel.LabelNameProfileType, Value: "cpu:type:unit:type:unit"},
				{Name: phlaremodel.LabelNameServiceNamePrivate, Value: "service_name"},
				{Name: model.MetricNameLabel, Value: "cpu"},
				{Name: phlaremodel.LabelNamePeriodType, Value: "type"},
				{Name: phlaremodel.LabelNamePeriodUnit, Value: "unit"},
				{Name: phlaremodel.LabelNameType, Value: "type"},
				{Name: phlaremodel.LabelNameUnit, Value: "unit"},
				{Name: phlaremodel.LabelNameServiceName, Value: "service_name"},
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			result, fps := CreateProfileLabels(true, newProfileFoo(), tt.in...)
			require.Equal(t, tt.expected, result[0])
			require.Equal(t, model.Fingerprint(tt.expected.Hash()), fps[0])
		})
	}
}

func newProfileFoo() *profilev1.Profile {
	return &profilev1.Profile{
		StringTable: append([]string{""}, []string{"unit", "type"}...),
		PeriodType: &profilev1.ValueType{
			Unit: 1,
			Type: 2,
		},
		SampleType: []*profilev1.ValueType{{
			Unit: 1,
			Type: 2,
		}},
	}
}
