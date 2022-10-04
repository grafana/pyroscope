package speedscope

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type mockIngester struct{ actual []*storage.PutInput }

func (m *mockIngester) Put(_ context.Context, p *storage.PutInput) error {
	m.actual = append(m.actual, p)
	return nil
}

var _ = Describe("Speedscope", func() {
	It("Can parse an event-format profile", func() {
		data, err := os.ReadFile("testdata/simple.speedscope.json")
		Expect(err).ToNot(HaveOccurred())

		key, err := segment.ParseKey("foo")
		Expect(err).ToNot(HaveOccurred())

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{Key: key}
		err = profile.Parse(context.Background(), ingester, nil, md)
		Expect(err).ToNot(HaveOccurred())

		Expect(ingester.actual).To(HaveLen(1))
		input := ingester.actual[0]

		Expect(input.Units).To(Equal(metadata.SamplesUnits))
		Expect(input.Key.Normalized()).To(Equal("foo{}"))
		expectedResult := `a;b 5
a;b;c 5
a;b;d 4
`
		Expect(input.Val.String()).To(Equal(expectedResult))
	})
})
