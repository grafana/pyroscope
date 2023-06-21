package phlaredb

import (
	"sort"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/phlare/pkg/model"
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
				{Name: model.MetricNameLabel, Value: "cpu"},
				{Name: phlaremodel.LabelNameUnit, Value: "unit"},
				{Name: phlaremodel.LabelNameProfileType, Value: "cpu:type:unit:type:unit"},
				{Name: phlaremodel.LabelNameType, Value: "type"},
				{Name: phlaremodel.LabelNamePeriodType, Value: "type"},
				{Name: phlaremodel.LabelNamePeriodUnit, Value: "unit"},
			},
		},
		{
			"with service_name",
			phlaremodel.Labels{
				{Name: model.MetricNameLabel, Value: "cpu"},
				{Name: phlaremodel.LabelNameServiceName, Value: "service_name"},
			},
			phlaremodel.Labels{
				{Name: model.MetricNameLabel, Value: "cpu"},
				{Name: phlaremodel.LabelNameUnit, Value: "unit"},
				{Name: phlaremodel.LabelNameProfileType, Value: "cpu:type:unit:type:unit"},
				{Name: phlaremodel.LabelNameType, Value: "type"},
				{Name: phlaremodel.LabelNamePeriodType, Value: "type"},
				{Name: phlaremodel.LabelNamePeriodUnit, Value: "unit"},
				{Name: labelNameServiceName, Value: "service_name"},
				{Name: phlaremodel.LabelNameServiceName, Value: "service_name"},
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(tt.expected)
			result, fps := labelsForProfile(newProfileFoo(), tt.in...)
			require.Equal(t, tt.expected, result[0])
			require.Equal(t, model.Fingerprint(tt.expected.Hash()), fps[0])
		})
	}
}
