package pprof

import (
	"context"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type mockIngester struct{ actual []*storage.PutInput }

func (m *mockIngester) Put(_ context.Context, p *storage.PutInput) error {
	m.actual = append(m.actual, p)
	return nil
}

var _ = Describe("pprof parsing", func() {
	Context("Go", func() {
		It("can parse CPU profiles", func() {
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

			w := NewParser(ParserConfig{
				Putter:      ingester,
				SampleTypes: tree.DefaultSampleTypeMapping,
				Labels:      labels,
				SpyName:     spyName,
			})

			err = w.Convert(context.Background(), start, end, p, false)
			Expect(err).ToNot(HaveOccurred())

			Expect(ingester.actual).To(HaveLen(1))
			input := ingester.actual[0]
			Expect(input.SpyName).To(Equal(spyName))
			Expect(input.StartTime).To(Equal(start))
			Expect(input.EndTime).To(Equal(end))
			Expect(input.SampleRate).To(Equal(uint32(100)))
			Expect(input.Val.Samples()).To(Equal(uint64(47)))
			Expect(input.Key.Normalized()).To(Equal("app.cpu{foo=bar}"))
			Expect(input.Val.String()).To(ContainSubstring("runtime.main;main.main;main.slowFunction;main.work 1"))
		})
	})

	Context("JS", func() {
		It("can parse CPU profile", func() {
			p, err := readPprofFixture("testdata/nodejs-wall.pb.gz")
			Expect(err).ToNot(HaveOccurred())

			ingester := new(mockIngester)
			spyName := "nodespy"
			now := time.Now()
			start := now
			end := now.Add(10 * time.Second)
			labels := map[string]string{
				"__name__": "app",
				"foo":      "bar",
			}

			w := NewParser(ParserConfig{
				Putter:      ingester,
				SampleTypes: tree.DefaultSampleTypeMapping,
				Labels:      labels,
				SpyName:     spyName,
			})

			err = w.Convert(context.Background(), start, end, p, false)
			Expect(err).ToNot(HaveOccurred())

			Expect(ingester.actual).To(HaveLen(1))
			input := ingester.actual[0]
			Expect(input.SpyName).To(Equal(spyName))
			Expect(input.StartTime).To(Equal(start))
			Expect(input.EndTime).To(Equal(end))
			Expect(input.SampleRate).To(Equal(uint32(100)))
			Expect(input.Val.Samples()).To(Equal(uint64(898)))
			Expect(input.Key.Normalized()).To(Equal("app.cpu{foo=bar}"))
			Expect(input.Val.String()).To(ContainSubstring("node:_http_server:resOnFinish:819;node:_http_server:detachSocket:252 1"))
		})

		It("can parse heap profiles", func() {
			p, err := readPprofFixture("testdata/nodejs-heap.pb.gz")
			Expect(err).ToNot(HaveOccurred())

			ingester := new(mockIngester)
			spyName := "nodespy"
			now := time.Now()
			start := now
			end := now.Add(10 * time.Second)
			labels := map[string]string{
				"__name__": "app",
				"foo":      "bar",
			}

			Expect(tree.DefaultSampleTypeMapping["inuse_objects"].Cumulative).To(BeFalse())
			Expect(tree.DefaultSampleTypeMapping["inuse_space"].Cumulative).To(BeFalse())
			tree.DefaultSampleTypeMapping["inuse_objects"].Cumulative = false
			tree.DefaultSampleTypeMapping["inuse_space"].Cumulative = false

			w := NewParser(ParserConfig{
				Putter:      ingester,
				SampleTypes: tree.DefaultSampleTypeMapping,
				Labels:      labels,
				SpyName:     spyName,
			})

			err = w.Convert(context.Background(), start, end, p, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(ingester.actual).To(HaveLen(2))
			sort.Slice(ingester.actual, func(i, j int) bool {
				return ingester.actual[i].Key.Normalized() < ingester.actual[j].Key.Normalized()
			})

			input := ingester.actual[0]
			Expect(input.SpyName).To(Equal(spyName))
			Expect(input.StartTime).To(Equal(start))
			Expect(input.EndTime).To(Equal(end))
			Expect(input.Val.Samples()).To(Equal(uint64(100498)))
			Expect(input.Key.Normalized()).To(Equal("app.inuse_objects{foo=bar}"))
			Expect(input.Val.String()).To(ContainSubstring("node:internal/streams/readable:readableAddChunk:236 138"))

			input = ingester.actual[1]
			Expect(input.SpyName).To(Equal(spyName))
			Expect(input.StartTime).To(Equal(start))
			Expect(input.EndTime).To(Equal(end))
			Expect(input.Val.Samples()).To(Equal(uint64(8357762)))
			Expect(input.Key.Normalized()).To(Equal("app.inuse_space{foo=bar}"))
			Expect(input.Val.String()).To(ContainSubstring("node:internal/net:isIPv6:35;:test:0 555360"))
		})
	})

	Context("pprof", func() {
		It("can parse uncompressed protobuf", func() {
			_, err := readPprofFixture("testdata/heap.pb")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("pprof parser", func() {
	p, err := readPprofFixture("testdata/cpu-exemplars.pb.gz")
	Expect(err).ToNot(HaveOccurred())

	m := make(map[string]*storage.PutInput)
	var skipExemplars bool

	JustBeforeEach(func() {
		putter := new(mockIngester)
		now := time.Now()
		start := now
		end := now.Add(10 * time.Second)

		w := NewParser(ParserConfig{
			Putter:        putter,
			Labels:        map[string]string{"__name__": "app"},
			SampleTypes:   tree.DefaultSampleTypeMapping,
			SkipExemplars: skipExemplars,
		})

		err = w.Convert(context.Background(), start, end, p, false)
		Expect(err).ToNot(HaveOccurred())
		m = make(map[string]*storage.PutInput)
		for _, x := range putter.actual {
			m[x.Key.Normalized()] = x
		}
	})

	expectBaselineProfiles := func(m map[string]*storage.PutInput) {
		baseline, ok := m["app.cpu{foo=bar}"]
		Expect(ok).To(BeTrue())
		Expect(baseline.Val.Samples()).To(Equal(uint64(49)))

		baseline, ok = m["app.cpu{foo=bar,function=fast}"]
		Expect(ok).To(BeTrue())
		Expect(baseline.Val.Samples()).To(Equal(uint64(150)))

		baseline, ok = m["app.cpu{foo=bar,function=slow}"]
		Expect(ok).To(BeTrue())
		Expect(baseline.Val.Samples()).To(Equal(uint64(674)))
	}

	expectExemplarProfiles := func(m map[string]*storage.PutInput) {
		exemplar, ok := m["app.cpu{foo=bar,function=slow,profile_id=72bee0038027cfb1}"]
		Expect(ok).To(BeTrue())
		Expect(exemplar.Val.Samples()).To(Equal(uint64(3)))

		exemplar, ok = m["app.cpu{foo=bar,function=fast,profile_id=ff4d0449f061174f}"]
		Expect(ok).To(BeTrue())
		Expect(exemplar.Val.Samples()).To(Equal(uint64(1)))
	}

	Context("by default exemplars are not skipped", func() {
		It("can parse all exemplars", func() {
			Expect(len(m)).To(Equal(435))
		})

		It("correctly handles labels and values", func() {
			expectExemplarProfiles(m)
		})

		It("merges baseline profiles", func() {
			expectBaselineProfiles(m)
		})
	})

	Context("when configured to skip exemplars", func() {
		BeforeEach(func() {
			skipExemplars = true
		})

		It("skip exemplars", func() {
			Expect(len(m)).To(Equal(3))
		})

		It("merges baseline profiles", func() {
			expectBaselineProfiles(m)
		})
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

		w := NewParser(ParserConfig{
			Putter: ingester,
			SampleTypes: map[string]*tree.SampleTypeConfig{
				"objects": {
					Units:       "objects",
					Aggregation: "average",
				},
				"space": {
					Units:       "bytes",
					Aggregation: "average",
				},
			},
			Labels:  labels,
			SpyName: spyName,
		})

		err = w.Convert(context.TODO(), start, end, p, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(ingester.actual).To(HaveLen(2))
		sort.Slice(ingester.actual, func(i, j int) bool {
			return ingester.actual[i].Key.Normalized() < ingester.actual[j].Key.Normalized()
		})

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
