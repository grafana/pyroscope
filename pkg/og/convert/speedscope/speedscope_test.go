package speedscope

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"

	"github.com/grafana/pyroscope/pkg/og/storage"
)

type mockIngester struct{ actual []*storage.PutInput }

func (m *mockIngester) Put(_ context.Context, p *storage.PutInput) error {
	m.actual = append(m.actual, p)
	return nil
}

func findInputByLabel(inputs []*storage.PutInput, normalizedLabel string) *storage.PutInput {
	for _, in := range inputs {
		if in.LabelSet.Normalized() == normalizedLabel {
			return in
		}
	}
	return nil
}

var _ = Describe("Speedscope", func() {
	It("Can parse an event-format profile", func() {
		data, err := os.ReadFile("testdata/simple.speedscope.json")
		Expect(err).ToNot(HaveOccurred())

		key, err := labelset.Parse("foo")
		Expect(err).ToNot(HaveOccurred())

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		Expect(err).ToNot(HaveOccurred())

		Expect(ingester.actual).To(HaveLen(1))
		input := ingester.actual[0]

		Expect(input.Units).To(Equal(metadata.SamplesUnits))
		Expect(input.LabelSet.Normalized()).To(Equal("foo{profile_name=simple.txt}"))
		expectedResult := `a;b 500
a;b;c 500
a;b;d 400
`
		Expect(input.Val.String()).To(Equal(expectedResult))
		Expect(input.SampleRate).To(Equal(uint32(10000)))
	})

	It("Can parse a sample-format profile", func() {
		data, err := os.ReadFile("testdata/two-sampled.speedscope.json")
		Expect(err).ToNot(HaveOccurred())

		key, err := labelset.Parse("foo{x=y}")
		Expect(err).ToNot(HaveOccurred())

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		Expect(err).ToNot(HaveOccurred())

		Expect(ingester.actual).To(HaveLen(2))

		input := findInputByLabel(ingester.actual, "foo.seconds{profile_name=one,x=y}")
		Expect(input).ToNot(BeNil())
		Expect(input.Units).To(Equal(metadata.SamplesUnits))
		Expect(input.LabelSet.Normalized()).To(Equal("foo.seconds{profile_name=one,x=y}"))
		expectedResult := `a;b 500
a;b;c 500
a;b;d 400
`
		Expect(input.Val.String()).To(Equal(expectedResult))
		Expect(input.SampleRate).To(Equal(uint32(100)))

		input2 := findInputByLabel(ingester.actual, "foo.seconds{profile_name=two,x=y}")
		Expect(input2).ToNot(BeNil())
		Expect(input2.Units).To(Equal(metadata.SamplesUnits))
		Expect(input2.LabelSet.Normalized()).To(Equal("foo.seconds{profile_name=two,x=y}"))
		Expect(input2.Val.String()).To(Equal(expectedResult))
		Expect(input2.SampleRate).To(Equal(uint32(100)))
	})

	It("Merges duplicate profiles", func() {
		data, err := os.ReadFile("testdata/duplicates.speedscope.json")
		Expect(err).ToNot(HaveOccurred())

		key, err := labelset.Parse("foo{x=y}")
		Expect(err).ToNot(HaveOccurred())

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		Expect(err).ToNot(HaveOccurred())

		// Three profiles merged in to one
		Expect(ingester.actual).To(HaveLen(1))

		input := ingester.actual[0]
		Expect(input.Units).To(Equal(metadata.SamplesUnits))
		// Note that profiles with different `endValue`s are merged
		// since `endValue` not represented in pprof
		Expect(input.LabelSet.Normalized()).To(Equal("foo{profile_name=one,x=y}"))
		expectedResult := `a;b 1500
a;b;c 1500
a;b;d 1200
`
		Expect(input.Val.String()).To(Equal(expectedResult))
		Expect(input.SampleRate).To(Equal(uint32(100)))
	})
})
