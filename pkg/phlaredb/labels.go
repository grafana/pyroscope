package phlaredb

import (
	"fmt"
	"strings"

	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
)

var labelNameServiceName = fmt.Sprintf("__%s__", phlaremodel.LabelNameServiceName)

func labelsForProfile(p *profilev1.Profile, externalLabels ...*typesv1.LabelPair) ([]phlaremodel.Labels, []model.Fingerprint) {
	// build label set per sample type before references are rewritten
	var (
		sb                                             strings.Builder
		lbls                                           = phlaremodel.NewLabelsBuilder(externalLabels)
		sampleType, sampleUnit, periodType, periodUnit string
		metricName                                     = phlaremodel.Labels(externalLabels).Get(model.MetricNameLabel)
	)

	// Inject into labels the __service_name__ label if it exists
	// This allows better locality of the data in parquet files (row group are sorted by).
	if serviceName := lbls.Labels().Get(phlaremodel.LabelNameServiceName); serviceName != "" {
		lbls.Set(labelNameServiceName, serviceName)
	}

	// set common labels
	if p.PeriodType != nil {
		periodType = p.StringTable[p.PeriodType.Type]
		lbls.Set(phlaremodel.LabelNamePeriodType, periodType)
		periodUnit = p.StringTable[p.PeriodType.Unit]
		lbls.Set(phlaremodel.LabelNamePeriodUnit, periodUnit)
	}

	profilesLabels := make([]phlaremodel.Labels, len(p.SampleType))
	seriesRefs := make([]model.Fingerprint, len(p.SampleType))
	for pos := range p.SampleType {
		sampleType = p.StringTable[p.SampleType[pos].Type]
		lbls.Set(phlaremodel.LabelNameType, sampleType)
		sampleUnit = p.StringTable[p.SampleType[pos].Unit]
		lbls.Set(phlaremodel.LabelNameUnit, sampleUnit)

		sb.Reset()
		_, _ = sb.WriteString(metricName)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(sampleType)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(sampleUnit)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(periodType)
		_, _ = sb.WriteRune(':')
		_, _ = sb.WriteString(periodUnit)
		t := sb.String()
		lbls.Set(phlaremodel.LabelNameProfileType, t)
		lbs := lbls.Labels().Clone()
		profilesLabels[pos] = lbs
		seriesRefs[pos] = model.Fingerprint(lbs.Hash())

	}
	return profilesLabels, seriesRefs
}
