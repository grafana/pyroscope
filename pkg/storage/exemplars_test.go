//go:build !windows
// +build !windows

package storage

import (
	"context"
	"encoding/hex"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("exemplars", func() {
	st := time.Now()
	et := st.Add(10 * time.Second)

	put := func(s *Storage, m map[string]string) {
		tree := tree.New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		Expect(s.Put(context.TODO(), &PutInput{
			StartTime:       st,
			EndTime:         et,
			Key:             segment.NewKey(m),
			Val:             tree.Clone(big.NewRat(1, 1)),
			SpyName:         "debugspy",
			SampleRate:      100,
			Units:           metadata.SamplesUnits,
			AggregationType: metadata.AverageAggregationType,
		})).ToNot(HaveOccurred())
	}

	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
			Expect(err).ToNot(HaveOccurred())

			put(s, map[string]string{
				"__name__":  "app.cpu",
				"span_name": "foo",
				// w/o profile_id, just to create the segment.
			})
			put(s, map[string]string{
				"__name__":   "app.cpu",
				"span_name":  "foo",
				"profile_id": "a",
			})
			put(s, map[string]string{
				"__name__":   "app.cpu",
				"span_name":  "foo",
				"profile_id": "a",
			})
			put(s, map[string]string{
				"__name__":   "app.cpu",
				"span_name":  "foo",
				"profile_id": "b",
			})

			s.exemplars.Sync()
		})

		Context("Get", func() {
			It("merges profiling data correctly", func() {
				defer s.Close()

				o, err := s.Get(context.Background(), &GetInput{
					Query: &flameql.Query{
						AppName: "app.cpu",
						Matchers: []*flameql.TagMatcher{
							{Key: segment.ProfileIDLabelName, Value: "a", Op: flameql.OpEqual},
							{Key: segment.ProfileIDLabelName, Value: "b", Op: flameql.OpEqual},
						},
					},
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(4)))
				Expect(o.Count).To(Equal(uint64(2)))
				Expect(o.SpyName).To(Equal("debugspy"))
				Expect(o.SampleRate).To(Equal(uint32(100)))
				Expect(o.Units).To(Equal(metadata.SamplesUnits))
				Expect(o.AggregationType).To(Equal(metadata.AverageAggregationType))
			})
		})

		Context("GetExemplar", func() {
			It("fetches exemplar data correctly", func() {
				defer s.Close()

				o, err := s.GetExemplar(context.Background(), GetExemplarInput{
					AppName:   "app.cpu",
					ProfileID: "a",
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(6)))

				Expect(o.StartTime).Should(BeTemporally("~", st, time.Second))
				Expect(o.EndTime).Should(BeTemporally("~", et, time.Second))

				Expect(o.Labels).To(Equal(map[string]string{"span_name": "foo"}))

				Expect(o.SpyName).To(Equal("debugspy"))
				Expect(o.SampleRate).To(Equal(uint32(100)))
				Expect(o.Units).To(Equal(metadata.SamplesUnits))
				Expect(o.AggregationType).To(Equal(metadata.AverageAggregationType))
			})
		})

		Context("MergeExemplars", func() {
			It("merges profiling data correctly", func() {
				defer s.Close()

				o, err := s.MergeExemplars(context.Background(), MergeExemplarsInput{
					AppName:    "app.cpu",
					ProfileIDs: []string{"a"},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(6)))

				o, err = s.MergeExemplars(context.Background(), MergeExemplarsInput{
					AppName:    "app.cpu",
					ProfileIDs: []string{"b"},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(3)))

				o, err = s.MergeExemplars(context.Background(), MergeExemplarsInput{
					AppName:    "app.cpu",
					ProfileIDs: []string{"a", "b"},
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(4)))
				Expect(o.Count).To(Equal(uint64(2)))
				Expect(o.SpyName).To(Equal("debugspy"))
				Expect(o.SampleRate).To(Equal(uint32(100)))
				Expect(o.Units).To(Equal(metadata.SamplesUnits))
				Expect(o.AggregationType).To(Equal(metadata.AverageAggregationType))
			})
		})
	})
})

var _ = Describe("Exemplars retention policy", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
			Expect(err).ToNot(HaveOccurred())
		})
		Context("when time-based retention policy is defined", func() {
			It("removes profiling data outside the period", func() {
				defer s.Close()

				tree := tree.New()
				tree.Insert([]byte("a;b"), uint64(1))
				tree.Insert([]byte("a;c"), uint64(2))

				k1, _ := segment.ParseKey("app.cpu{profile_id=a}")
				t1 := time.Now()
				t2 := t1.Add(10 * time.Second)
				Expect(s.Put(context.TODO(), &PutInput{
					StartTime: t1,
					EndTime:   t2,
					Key:       k1,
					Val:       tree.Clone(big.NewRat(1, 1)),
				})).ToNot(HaveOccurred())

				t3 := t2.Add(10 * time.Second)
				t4 := t3.Add(10 * time.Second)
				k2, _ := segment.ParseKey("app.cpu{profile_id=b}")
				Expect(s.Put(context.TODO(), &PutInput{
					StartTime: t3,
					EndTime:   t4,
					Key:       k2,
					Val:       tree.Clone(big.NewRat(1, 1)),
				})).ToNot(HaveOccurred())

				// Just to create the segment.
				k3, _ := segment.ParseKey("app.cpu{}")
				Expect(s.Put(context.TODO(), &PutInput{
					StartTime: t3,
					EndTime:   t4,
					Key:       k3,
					Val:       tree.Clone(big.NewRat(1, 1)),
				})).ToNot(HaveOccurred())
				s.exemplars.Sync()
				rp := &segment.RetentionPolicy{ExemplarsRetentionTime: t3}
				s.exemplars.enforceRetentionPolicy(context.Background(), rp)

				o, err := s.MergeExemplars(context.Background(), MergeExemplarsInput{
					AppName:    "app.cpu",
					ProfileIDs: []string{"a", "b"},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(3)))

				gi := new(GetInput)
				gi.Query, _ = flameql.ParseQuery(`app.cpu{profile_id="b"}`)
				o2, err := s.Get(context.TODO(), gi)
				Expect(err).ToNot(HaveOccurred())
				Expect(o2.Tree).ToNot(BeNil())
				Expect(o2.Tree.Samples()).To(Equal(uint64(3)))
			})
		})
	})
})

func randomBytesHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

var _ = Describe("Concurrent exemplars insertion", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
			Expect(err).ToNot(HaveOccurred())
		})
		Context("when exemplars ingested concurrently", func() {
			It("does not race with sync and periodic flush", func() {
				defer s.Close()
				const (
					n = 4
					c = 100
				)

				stop := make(chan struct{})
				done := make(chan struct{})
				go func() {
					defer close(done)
					for {
						select {
						case <-stop:
							return
						case <-time.After(10 * time.Millisecond):
							s.exemplars.Sync()
						}
					}
				}()

				var wg sync.WaitGroup
				wg.Add(n)

				for i := 0; i < n; i++ {
					go func() {
						defer wg.Done()
						for j := 0; j < c; j++ {
							tree := tree.New()
							tree.Insert([]byte("a;b"), uint64(1))
							tree.Insert([]byte("a;c"), uint64(2))
							Expect(s.Put(context.TODO(), &PutInput{
								StartTime: testing.SimpleTime(0),
								EndTime:   testing.SimpleTime(30),
								Val:       tree,
								Key: segment.NewKey(map[string]string{
									"__name__":   "app.cpu",
									"profile_id": randomBytesHex(8),
								}),
							})).ToNot(HaveOccurred())
						}
					}()
				}

				wg.Wait()
				close(stop)
				<-done
			})
		})
	})
})

var _ = Describe("Exemplar serialization", func() {
	Context("exemplars serialisation", func() {
		It("can be serialized and deserialized", func() {
			const appName = "app.cpu"
			profileID := randomBytesHex(8)
			t := tree.New()
			t.Insert([]byte("a;b"), uint64(1))
			t.Insert([]byte("a;c"), uint64(2))

			e := exemplarEntry{
				Key:       exemplarKey(appName, profileID),
				AppName:   appName,
				ProfileID: profileID,

				StartTime: testing.SimpleTime(123).UnixNano(),
				EndTime:   testing.SimpleTime(456).UnixNano(),
				Tree:      t,
				Labels: map[string]string{
					"__name__":   appName,
					"profile_id": profileID,
					"foo":        "bar",
					"baz":        "qux",
				},
			}

			d := dict.New()
			b, err := e.Serialize(d, 1<<10)
			Expect(err).ToNot(HaveOccurred())

			var n exemplarEntry
			Expect(n.Deserialize(d, b)).ToNot(HaveOccurred())

			Expect(e.StartTime).To(Equal(n.StartTime))
			Expect(e.EndTime).To(Equal(n.EndTime))
			Expect(e.Tree.String()).To(Equal(n.Tree.String()))
			Expect(n.Labels).To(Equal(map[string]string{
				"foo": "bar",
				"baz": "qux",
			}))
		})
	})

	Context("exemplars v1 compatibility", func() {
		It("can deserialize exemplars v1", func() {
			b, err := os.ReadFile("./testdata/exemplar.v1.bin")
			Expect(err).ToNot(HaveOccurred())

			var n exemplarEntry
			Expect(n.Deserialize(dict.New(), b)).ToNot(HaveOccurred())

			Expect(n.Tree.Samples()).To(Equal(uint64(81255)))
			Expect(n.StartTime).To(BeZero())
			Expect(n.EndTime).To(BeZero())
			Expect(n.Labels).To(BeNil())
		})
	})
})
