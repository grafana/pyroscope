package profile

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
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

var _ = Describe("metrics exporter", func() {
	var (
		exporter *mockExporter
		ingester *mockIngester

		md ingestion.Metadata
		p  RawProfile
	)

	JustBeforeEach(func() {
		ingester = new(mockIngester)
		md = ingestion.Metadata{Key: new(segment.Key)}
		p = RawProfile{
			Format:  ingestion.FormatGroups,
			RawData: []byte("foo;bar 1\nfoo;baz 2\n"),
		}

		Expect(p.Parse(context.Background(), ingester, exporter, md)).ToNot(HaveOccurred())
	})

	ItIngestsTree := func() {
		Expect(ingester.actual[0].Val.Samples()).To(Equal(uint64(3)))
	}

	Context("if evaluation successful", func() {
		BeforeEach(func() {
			exporter = newMockExporter(true)
		})
		It("ingests the tree", ItIngestsTree)
		It("observes stack values", func() {
			Expect(exporter.observer.keys).To(Equal([]string{"foo;bar", "foo;baz"}))
			Expect(exporter.observer.values).To(Equal([]int{1, 2}))
		})
	})

	Context("if evaluation unsuccessful", func() {
		BeforeEach(func() {
			exporter = newMockExporter(false)
		})
		It("ingests the tree", ItIngestsTree)
		It("does not observe stack values", func() {
			Expect(exporter.observer).To(BeNil())
		})
	})
})
