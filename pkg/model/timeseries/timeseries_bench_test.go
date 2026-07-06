package timeseries

import (
	"fmt"
	"testing"

	"github.com/prometheus/common/model"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

func BenchmarkBuilderAdd(b *testing.B) {
	const numSeries = 20
	labelSets := make([]phlaremodel.Labels, numSeries)
	for i := range labelSets {
		labelSets[i] = phlaremodel.LabelsFromStrings(
			"__name__", "process_cpu",
			"service_name", fmt.Sprintf("service-%d", i),
			"pod", fmt.Sprintf("pod-%d", i),
			"namespace", "default",
		)
	}

	b.ReportAllocs()
	builder := NewBuilder("service_name")
	for i := 0; i < b.N; i++ {
		lbs := labelSets[i%numSeries]
		builder.Add(model.Fingerprint(i%numSeries), lbs, int64(i)*15000, 1.0, schemav1.Annotations{}, "")
	}
	if len(builder.Build()) == 0 {
		b.Fatal("no series built")
	}
}
