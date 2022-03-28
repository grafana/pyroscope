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

var _ = Describe("pprof profile_id multiplexing", func() {
	It("parses data correctly", func() {
		p, err := readPprofFixture("testdata/cpu-mux.pb.gz")
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

		var actualTotal uint64
		const (
			expectedTotal = uint64(789)
			expectedDiff  = uint64(20)
		)

		for _, v := range ingester.actual {
			if v.Key.Normalized() == "app.cpu{foo=bar}" {
				Expect(v.Val.Samples()).To(Equal(expectedTotal))
				continue
			}
			actualTotal += v.Val.Samples()
		}

		Expect(expectedTotal - actualTotal).To(Equal(expectedDiff))
	})
})

var _ = Describe("custom pprof parsing", func() {
	It("parses data correctly", func() {
		p, err := readPprofFixture("testdata/heap-js.pprof")
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

		w := NewProfileWriter(ingester, labels, map[string]*tree.SampleTypeConfig{
			"objects": {
				Units:       "objects",
				Aggregation: "avg",
			},
			"space": {
				Units:       "bytes",
				Aggregation: "avg",
			},
		})

		err = w.WriteProfile(start, end, spyName, p)
		Expect(err).ToNot(HaveOccurred())

		Expect(ingester.actual).To(HaveLen(2))

		input := ingester.actual[0]
		Expect(input.SpyName).To(Equal(spyName))
		Expect(input.StartTime).To(Equal(start))
		Expect(input.EndTime).To(Equal(end))
		Expect(input.Val.Samples()).To(Equal(uint64(66148)))
		Expect(input.Key.Normalized()).To(Equal("app.objects{foo=bar}"))
		Expect(input.Val.String()).To(ContainSubstring("parserOnHeadersComplete;parserOnIncoming 2428"))

		input = ingester.actual[1]
		Expect(input.SpyName).To(Equal(spyName))
		Expect(input.StartTime).To(Equal(start))
		Expect(input.EndTime).To(Equal(end))
		Expect(input.Val.Samples()).To(Equal(uint64(6388384)))
		Expect(input.Key.Normalized()).To(Equal("app.space{foo=bar}"))
		Expect(input.Val.String()).To(ContainSubstring("parserOnHeadersComplete;parserOnIncoming 524448"))
	})
})
