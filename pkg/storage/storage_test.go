package storage

import (
	"runtime"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/mem"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

// 21:22:08      air |  (time.Duration) 16m40s,
// 21:22:08      air |  (time.Duration) 2h46m40s,
// 21:22:08      air |  (time.Duration) 27h46m40s,
// 21:22:08      air |  (time.Duration) 277h46m40s,
// 21:22:08      air |  (time.Duration) 2777h46m40s,
// 21:22:08      air |  (time.Duration) 27777h46m40s

var s *Storage

var _ = Describe("storage package", func() {
	logrus.SetLevel(logrus.InfoLevel)

	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			evictInterval = 2 * time.Second

			var err error
			s, err = New(&(*cfg).Server, prometheus.NewRegistry())
			Expect(err).ToNot(HaveOccurred())
		})

		Context("delete tests", func() {
			Context("simple delete", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(1))
					tree.Insert([]byte("a;c"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := segment.ParseKey("foo")

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					err := s.Delete(&DeleteInput{
						Key: key,
					})
					Expect(err).ToNot(HaveOccurred())

					gOut, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(gOut).To(BeNil())
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("delete all trees", func() {
				It("works correctly", func() {
					tree1 := tree.New()
					tree1.Insert([]byte("a;b"), uint64(1))
					tree1.Insert([]byte("a;c"), uint64(2))
					tree2 := tree.New()
					tree2.Insert([]byte("c;d"), uint64(1))
					tree2.Insert([]byte("e;f"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := segment.ParseKey("foo")

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree1,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree2,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					err := s.Delete(&DeleteInput{
						Key: key,
					})
					Expect(err).ToNot(HaveOccurred())

					gOut, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(gOut).To(BeNil())
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("put after delete", func() {
				It("works correctly", func() {
					tree1 := tree.New()
					tree1.Insert([]byte("a;b"), uint64(1))
					tree1.Insert([]byte("a;c"), uint64(2))
					tree2 := tree.New()
					tree2.Insert([]byte("c;d"), uint64(1))
					tree2.Insert([]byte("e;f"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := segment.ParseKey("foo")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree1,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					err = s.Delete(&DeleteInput{
						Key: key,
					})
					Expect(err).ToNot(HaveOccurred())

					s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree2,
						SpyName:    "testspy",
						SampleRate: 100,
					})

					gOut, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(gOut.Tree).ToNot(BeNil())
					Expect(gOut.Tree.String()).To(Equal(tree2.String()))
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
		})

		Context("smoke tests", func() {
			Context("check segment cache", func() {
				It("works correctly", func() {
					tree := tree.New()

					size := 32
					treeKey := make([]byte, size)
					for i := 0; i < size; i++ {
						treeKey[i] = 'a'
					}
					for i := 0; i < 60; i++ {
						k := string(treeKey) + strconv.Itoa(i+1)
						tree.Insert([]byte(k), uint64(i+1))

						key, _ := segment.ParseKey("tree_key" + strconv.Itoa(i+1))
						err := s.Put(&PutInput{
							Key:        key,
							Val:        tree,
							SpyName:    "testspy",
							SampleRate: 100,
						})
						Expect(err).ToNot(HaveOccurred())
					}
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("simple 10 second write", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(1))
					tree.Insert([]byte("a;c"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := segment.ParseKey("foo")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					o, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(o.Tree).ToNot(BeNil())
					Expect(o.Tree.String()).To(Equal(tree.String()))
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("simple 20 second write", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(2))
					tree.Insert([]byte("a;c"), uint64(4))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(29)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)
					key, _ := segment.ParseKey("foo")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					o, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       key,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(o.Tree).ToNot(BeNil())
					Expect(o.Tree.String()).To(Equal(tree.String()))
					Expect(s.Close()).ToNot(HaveOccurred())
				})
			})
			Context("evict cache items periodically", func() {
				It("works correctly", func() {
					tree := tree.New()

					size := 16
					treeKey := make([]byte, size)
					for i := 0; i < size; i++ {
						treeKey[i] = 'a'
					}
					for i := 0; i < 200; i++ {
						k := string(treeKey) + strconv.Itoa(i+1)
						tree.Insert([]byte(k), uint64(i+1))

						key, _ := segment.ParseKey("tree_key" + strconv.Itoa(i+1))
						err := s.Put(&PutInput{
							Key:        key,
							Val:        tree,
							SpyName:    "testspy",
							SampleRate: 100,
						})
						Expect(err).ToNot(HaveOccurred())
					}

					for i := 0; i < 5; i++ {
						_, err := mem.VirtualMemory()
						Expect(err).ToNot(HaveOccurred())

						var m runtime.MemStats
						runtime.ReadMemStats(&m)
						time.Sleep(evictInterval)
					}
				})
			})
			Context("persist data between restarts", func() {
				It("works correctly", func() {
					tree := tree.New()
					tree.Insert([]byte("a;b"), uint64(1))
					tree.Insert([]byte("a;c"), uint64(2))
					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					st2 := testing.SimpleTime(0)
					et2 := testing.SimpleTime(30)

					appKey, _ := segment.ParseKey("foo")
					key, _ := segment.ParseKey("foo{tag=value}")

					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					o, err := s.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       appKey,
					})

					Expect(err).ToNot(HaveOccurred())
					Expect(o.Tree).ToNot(BeNil())
					Expect(o.Tree.String()).To(Equal(tree.String()))
					Expect(s.Close()).ToNot(HaveOccurred())

					s2, err := New(&(*cfg).Server, prometheus.NewRegistry())
					Expect(err).ToNot(HaveOccurred())

					o2, err := s2.Get(&GetInput{
						StartTime: st2,
						EndTime:   et2,
						Key:       appKey,
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(o2.Tree).ToNot(BeNil())
					Expect(o2.Tree.String()).To(Equal(tree.String()))
					Expect(s2.Close()).ToNot(HaveOccurred())
				})
			})
		})
	})
})

var _ = Describe("DeleteDataBefore", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(&(*cfg).Server, prometheus.NewRegistry())
			Expect(err).ToNot(HaveOccurred())
		})

		Context("simple case 1", func() {
			It("does not return errors", func() {
				tree := tree.New()
				tree.Insert([]byte("a;b"), uint64(1))
				tree.Insert([]byte("a;c"), uint64(2))
				st := time.Now().Add(time.Hour * 24 * 10 * -1)
				et := st.Add(time.Second * 10)
				key, _ := segment.ParseKey("foo")

				err := s.Put(&PutInput{
					StartTime:  st,
					EndTime:    et,
					Key:        key,
					Val:        tree,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(s.DeleteDataBefore(time.Now().Add(-1 * time.Hour))).ToNot(HaveOccurred())
				Expect(s.Close()).ToNot(HaveOccurred())
			})
		})

		Context("simple case 2", func() {
			It("does not return errors", func() {
				tree := tree.New()
				tree.Insert([]byte("a;b"), uint64(1))
				tree.Insert([]byte("a;c"), uint64(2))
				st := testing.SimpleTime(10)
				et := testing.SimpleTime(20)
				key, _ := segment.ParseKey("foo")

				err := s.Put(&PutInput{
					StartTime:  st,
					EndTime:    et,
					Key:        key,
					Val:        tree,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(s.DeleteDataBefore(time.Now().Add(-1 * time.Hour))).ToNot(HaveOccurred())
				Expect(s.Close()).ToNot(HaveOccurred())
			})
		})
	})
})
