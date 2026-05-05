package block

import (
	"io"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

func benchSeries(n int) []phlaremodel.Labels {
	out := make([]phlaremodel.Labels, n)
	for i := range out {
		out[i] = phlaremodel.LabelsFromStrings(
			phlaremodel.LabelNameServiceName, "svc",
			"__name__", "process_cpu",
			"pod", "pod-"+strconv.Itoa(i),
		)
	}
	return out
}

func BenchmarkDatasetIndexWriter_WriteTo(b *testing.B) {
	const datasetsPerTenant = 4
	const seriesPerDataset = 100
	series := benchSeries(seriesPerDataset)

	b.ReportAllocs()
	for b.Loop() {
		w := NewDatasetIndexWriter()
		for d := range datasetsPerTenant {
			w.SetIndex(uint32(d))
			for j, lbs := range series {
				w.AddSeries(lbs, model.Fingerprint(uint64(d)<<32|uint64(j)))
			}
		}
		if _, err := w.WriteTo(io.Discard); err != nil {
			b.Fatal(err)
		}
		w.Close()
	}
}
