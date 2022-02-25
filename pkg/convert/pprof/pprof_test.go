package pprof

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type mockIngester struct{ actual []*storage.PutInput }

func (m *mockIngester) Enqueue(p *storage.PutInput) { m.actual = append(m.actual, p) }

var _ = Describe("pprof parsing", func() {
	It("parses data correctly", func() {
		p, err := readPprofFixture("testdata/cpu.pb.gz")
		Expect(err).ToNot(HaveOccurred())

		ingester := new(mockIngester)
		spyName := "spy-name"
		now := time.Now()
		start := now
		end := now.Add(10 * time.Second)
		labels := map[string]string{
			"__name__": "app",
			"foo":      "bar",
		}

		w := NewProfileWriter(ingester, labels, tree.DefaultSampleTypeMapping)
		err = w.WriteProfile(start, end, spyName, p)
		Expect(err).ToNot(HaveOccurred())

		Expect(ingester.actual).To(HaveLen(1))
		input := ingester.actual[0]
		Expect(input.SpyName).To(Equal(spyName))
		Expect(input.StartTime).To(Equal(start))
		Expect(input.EndTime).To(Equal(end))
		Expect(input.Val.Samples()).To(Equal(uint64(47)))
		Expect(input.Key.Normalized()).To(Equal("app.cpu{foo=bar}"))
		Expect(input.Val.String()).To(ContainSubstring("runtime.main;main.main;main.slowFunction;main.work 1"))
	})
})
