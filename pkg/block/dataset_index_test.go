package block

import (
	"io"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
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

func TestDatasetIndexWriterSnapshotsLabels(t *testing.T) {
	labels := phlaremodel.LabelsFromStrings("service_name", "original")
	w := NewDatasetIndexWriter()
	t.Cleanup(w.Close)
	w.AddSeries(0, labels, 1)

	labels[0].Value = "mutated"
	require.Equal(t, "original", w.series[0].labels[0].value)
}

func TestCompactionIndexWritersShareImmutableLabels(t *testing.T) {
	source := phlaremodel.LabelsFromStrings("service_name", "original")
	entry := ProfileEntry{
		Fingerprint: 1,
		Row:         make(schemav1.ProfileRow, 4),
		labels:      newImmutableLabels(source),
	}
	index := newIndexRewriter()
	index.rewriteRow(entry)

	datasetIndex := NewDatasetIndexWriter()
	t.Cleanup(datasetIndex.Close)
	datasetIndex.addSeries(0, entry.labels, entry.Fingerprint)

	source[0].Value = "mutated"
	require.Equal(t, "original", index.series[0].labels[0].value)
	require.Equal(t, "original", datasetIndex.series[0].labels[0].value)
	require.Same(t, &index.series[0].labels[0], &datasetIndex.series[0].labels[0])
}

func BenchmarkDatasetIndexWriter_WriteTo(b *testing.B) {
	const datasetsPerTenant = 4
	const seriesPerDataset = 100
	series := benchSeries(seriesPerDataset)

	b.ReportAllocs()
	for b.Loop() {
		w := NewDatasetIndexWriter()
		for d := range datasetsPerTenant {
			for j, lbs := range series {
				w.AddSeries(uint32(d), lbs, model.Fingerprint(uint64(d)<<32|uint64(j)))
			}
		}
		if _, err := w.WriteTo(io.Discard); err != nil {
			b.Fatal(err)
		}
		w.Close()
	}
}
