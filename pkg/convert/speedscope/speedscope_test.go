package speedscope

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}
		input := storage.PutInput{}
		err = profile.convert(ingester, &input)
		Expect(err).ToNot(HaveOccurred())

		// TODO: test what was put
	})
})
