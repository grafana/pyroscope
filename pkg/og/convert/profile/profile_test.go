package profile

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/dskit/user"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/v2/pkg/og/ingestion"
	"github.com/grafana/pyroscope/v2/pkg/og/storage"
)

type mockLimits struct {
	maxSymbolLen int
	maxSamples   int
}

func (m mockLimits) MaxProfileSizeBytes(_ string) int         { return 0 }
func (m mockLimits) MaxProfileSymbolValueLength(_ string) int { return m.maxSymbolLen }
func (m mockLimits) MaxProfileStacktraceSamples(_ string) int { return m.maxSamples }

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

	require.NoError(t, p.Parse(context.Background(), ingester, exporter, md, nil))
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

// serializationExample is a valid tree-format payload (same fixture as serialize_nodict_test.go).
var serializationExample = []byte("\x00\x00\x01\x01a\x00\x02\x01b\x01\x00\x01c\x02\x00")

func TestFormatTreeWithLimits(t *testing.T) {
	t.Run("parses a valid tree payload and applies limits", func(t *testing.T) {
		ingester := new(mockIngester)
		md := ingestion.Metadata{LabelSet: new(labelset.LabelSet)}
		ctx := user.InjectOrgID(context.Background(), "test-tenant")
		p := RawProfile{Format: ingestion.FormatTree, RawData: serializationExample}
		err := p.Parse(ctx, ingester, newMockExporter(false), md, mockLimits{maxSymbolLen: 65535, maxSamples: 16000})
		require.NoError(t, err)
		require.Len(t, ingester.actual, 1)
	})

	t.Run("rejects a payload with an oversized name length when limits are set", func(t *testing.T) {
		// varint encoding of 0xFFFFFFFFFFFFFFFF — without bounds checks this panics
		payload := bytes.Repeat([]byte{0xff}, 9)
		payload = append(payload, 0x01)
		ingester := new(mockIngester)
		md := ingestion.Metadata{LabelSet: new(labelset.LabelSet)}
		ctx := user.InjectOrgID(context.Background(), "test-tenant")
		p := RawProfile{Format: ingestion.FormatTree, RawData: payload}
		err := p.Parse(ctx, ingester, newMockExporter(false), md, mockLimits{maxSymbolLen: 65535, maxSamples: 16000})
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds maximum")
	})

	t.Run("does not enforce limits when nil is passed", func(t *testing.T) {
		// With nil limits the oversized varint reaches make([]byte, n) and panics
		// without the serialize.go guard — the guard doesn't fire because maxNameLen=0
		// means disabled. This test confirms nil limits don't error on valid payloads.
		ingester := new(mockIngester)
		md := ingestion.Metadata{LabelSet: new(labelset.LabelSet)}
		ctx := user.InjectOrgID(context.Background(), "test-tenant")
		p := RawProfile{Format: ingestion.FormatTree, RawData: serializationExample}
		err := p.Parse(ctx, ingester, newMockExporter(false), md, nil)
		require.NoError(t, err)
	})
}
