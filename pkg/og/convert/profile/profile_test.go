package profile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/v2/pkg/og/ingestion"
	"github.com/grafana/pyroscope/v2/pkg/og/storage"
)

type mockIngester struct{ actual []*storage.PutInput }

func (m *mockIngester) Put(_ context.Context, p *storage.PutInput) error {
	m.actual = append(m.actual, p)
	return nil
}

type mockExporter struct {
	observe  bool
	observer *mockObserver
}

func newMockExporter(observe bool) *mockExporter { return &mockExporter{observe: observe} }

type mockObserver struct {
	keys   []string
	values []int
}

func (m *mockExporter) Evaluate(*storage.PutInput) (storage.SampleObserver, bool) {
	if !m.observe {
		return nil, false
	}
	m.observer = new(mockObserver)
	return m.observer, true
}

func (m *mockObserver) Observe(k []byte, v int) {
	m.keys = append(m.keys, string(k))
	m.values = append(m.values, v)
}

func runMetricsExporterTest(t *testing.T, observe bool) (*mockExporter, *mockIngester) {
	t.Helper()

	exporter := newMockExporter(observe)
	ingester := new(mockIngester)
	md := ingestion.Metadata{LabelSet: new(labelset.LabelSet)}
	p := RawProfile{
		Format:  ingestion.FormatGroups,
		RawData: []byte("foo;bar 1\nfoo;baz 2\n"),
	}

	require.NoError(t, p.Parse(context.Background(), ingester, exporter, md))
	return exporter, ingester
}

func TestMetricsExporter(t *testing.T) {
	t.Run("if evaluation successful", func(t *testing.T) {
		exporter, ingester := runMetricsExporterTest(t, true)

		require.Equal(t, uint64(3), ingester.actual[0].Val.Samples())
		require.Equal(t, []string{"foo;bar", "foo;baz"}, exporter.observer.keys)
		require.Equal(t, []int{1, 2}, exporter.observer.values)
	})

	t.Run("if evaluation unsuccessful", func(t *testing.T) {
		exporter, ingester := runMetricsExporterTest(t, false)

		require.Equal(t, uint64(3), ingester.actual[0].Val.Samples())
		require.Nil(t, exporter.observer)
	})
}
