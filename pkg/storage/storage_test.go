package storage

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/mem"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
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

var maxTime = time.Unix(1<<62, 999999999)

var _ = Describe("storage package", func() {
	logrus.SetLevel(logrus.InfoLevel)

	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry())
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

					Expect(s.Delete(&DeleteInput{key})).ToNot(HaveOccurred())
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

					Expect(s.Delete(&DeleteInput{key})).ToNot(HaveOccurred())
					s.GetValues("__name__", func(v string) bool {
						Fail("app name label was not removed")
						return false
					})

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

					Expect(s.Delete(&DeleteInput{key})).ToNot(HaveOccurred())
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
			Context("delete app by name", func() {
				It("works correctly", func() {
					appname := "my.app.cpu"

					// We insert info for an app
					tree1 := tree.New()
					tree1.Insert([]byte("a;b"), uint64(1))

					st := testing.SimpleTime(10)
					et := testing.SimpleTime(19)
					key, _ := segment.ParseKey(appname)
					err := s.Put(&PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree1,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())

					// These assertions are for my own sanity
					// And to make the test more clear

					s.trees.Dump()
					_ = fmt.Println

					// Let's flush that to disk
					// TODO do I need this?
					// supposedly i need to flush this bc it would serialize
					// and create a dictionary
					// however that doesn't seem to be working
					s.trees.Flush()

					/*************************/
					/*  D i m e n s i o n s  */
					/*************************/
					dimension, ok := s.lookupAppDimension(appname)
					// TODO is this enough?
					Expect(ok).To(BeTrue())

					/***************/
					/*  T r e e s  */
					/***************/
					// There's something in the tree cache
					// And that something is our brand new tree!
					Expect(s.trees.Cache.Size()).To(Equal(uint64(1)))
					depth := 0 // TODO for this case is it even possible to have a different depth?
					treeKeyName := key.TreeKey(depth, st)
					t, ok := s.trees.Cache.Lookup(treeKeyName)
					Expect(ok).To(BeTrue())
					Expect(t).To(Equal(tree1))

					/***************/
					/*  D i c t s  */
					/***************/

					// TODO there should be only one right????
					dimensionKey := dimension.Keys[0]
					segmentKey, err := segment.ParseKey(string(dimensionKey))
					Expect(err).ToNot(HaveOccurred())

					fmt.Println("dumping dicts")
					s.dicts.Dump()
					fmt.Println("finished dumping dicts")
					fmt.Println("key dictkey")
					fmt.Println(key.DictKey())
					fmt.Println("segmentKey", segmentKey)
					fmt.Println("dictKey", segmentKey.DictKey())

					Expect(s.dicts.Cache.Size()).To(Equal(uint64(1)))
					_, ok = s.dicts.Cache.Lookup(segmentKey.DictKey())

					Expect(ok).To(BeTrue())

					/*************************/
					/*  D e l e t e   a p p  */
					/*************************/
					err = s.DeleteApp(appname)
					Expect(err).ToNot(HaveOccurred())

					/***************/
					/*  T r e e s  */
					/***************/
					// Trees should've been deleted from CACHE
					Expect(s.trees.Cache.Size()).To(Equal(uint64(0)))
					_, ok = s.trees.Cache.Lookup(treeKeyName)
					Expect(ok).To(BeFalse())
					// Trees should've been also deleted from DISK
					// TODO: how to check for that?

					/*************************/
					/*  D i m e n s i o n s  */
					/*************************/
					_, ok = s.lookupAppDimension(appname)
					Expect(ok).To(BeFalse())

					/***************/
					/*  D i c t s  */
					/***************/
					_, ok = s.dicts.Cache.Lookup(key.DictKey())
					Expect(ok).To(BeFalse())

					// Now all this stuff should've been deleted
					// Trees should've been deleted
					// Both in cache and in disk?

					//					tree1.Insert([]byte("a;c"), uint64(2))

					//					tree2 := tree.New()
					//					tree2.Insert([]byte("c;d"), uint64(1))
					//					tree2.Insert([]byte("e;f"), uint64(2))
					//					st := testing.SimpleTime(10)
					//					et := testing.SimpleTime(19)
					//					st2 := testing.SimpleTime(0)
					//					et2 := testing.SimpleTime(30)
					//					key, _ := segment.ParseKey("foo")
					//
					//					err := s.Put(&PutInput{
					//						StartTime:  st,
					//						EndTime:    et,
					//						Key:        key,
					//						Val:        tree1,
					//						SpyName:    "testspy",
					//						SampleRate: 100,
					//					})
					//					Expect(err).ToNot(HaveOccurred())
					//
					//					Expect(s.Delete(&DeleteInput{key})).ToNot(HaveOccurred())
					//					s.Put(&PutInput{
					//						StartTime:  st,
					//						EndTime:    et,
					//						Key:        key,
					//						Val:        tree2,
					//						SpyName:    "testspy",
					//						SampleRate: 100,
					//					})
					//
					//					gOut, err := s.Get(&GetInput{
					//						StartTime: st2,
					//						EndTime:   et2,
					//						Key:       key,
					//					})
					//
					//					Expect(err).ToNot(HaveOccurred())
					//					Expect(gOut.Tree).ToNot(BeNil())
					//					Expect(gOut.Tree.String()).To(Equal(tree2.String()))
					//					Expect(s.Close()).ToNot(HaveOccurred())
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
						time.Sleep(time.Second)
					}
					Expect(s.Close()).ToNot(HaveOccurred())
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

					s2, err := New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry())
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

var _ = Describe("querying", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry())
			Expect(err).ToNot(HaveOccurred())
			keys := []string{
				"app.name{foo=bar,baz=qux}",
				"app.name{foo=bar,baz=xxx}",
				"app.name{waldo=fred,baz=xxx}",
			}
			for _, k := range keys {
				t := tree.New()
				t.Insert([]byte("a;b"), uint64(1))
				t.Insert([]byte("a;c"), uint64(2))
				st := testing.SimpleTime(10)
				et := testing.SimpleTime(19)
				key, err := segment.ParseKey(k)
				Expect(err).ToNot(HaveOccurred())
				err = s.Put(&PutInput{
					StartTime:  st,
					EndTime:    et,
					Key:        key,
					Val:        t,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		Context("basic queries", func() {
			It("get returns result with query", func() {
				qry, err := flameql.ParseQuery(`app.name{foo="bar"}`)
				Expect(err).ToNot(HaveOccurred())
				output, err := s.Get(&GetInput{
					StartTime: time.Time{},
					EndTime:   maxTime,
					Query:     qry,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(output).ToNot(BeNil())
				Expect(output.Tree).ToNot(BeNil())
				Expect(output.Tree.Samples()).To(Equal(uint64(6)))
				Expect(s.Close()).ToNot(HaveOccurred())
			})

			It("get returns a particular tree for a fully qualified key", func() {
				k, err := segment.ParseKey(`app.name{foo=bar,baz=qux}`)
				Expect(err).ToNot(HaveOccurred())
				output, err := s.Get(&GetInput{
					StartTime: time.Time{},
					EndTime:   maxTime,
					Key:       k,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(output).ToNot(BeNil())
				Expect(output.Tree).ToNot(BeNil())
				Expect(output.Tree.Samples()).To(Equal(uint64(3)))
				Expect(s.Close()).ToNot(HaveOccurred())
			})

			It("get returns all results for a key containing only app name", func() {
				k, err := segment.ParseKey(`app.name`)
				Expect(err).ToNot(HaveOccurred())
				output, err := s.Get(&GetInput{
					StartTime: time.Time{},
					EndTime:   maxTime,
					Key:       k,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(output).ToNot(BeNil())
				Expect(output.Tree).ToNot(BeNil())
				Expect(output.Tree.Samples()).To(Equal(uint64(9)))
				Expect(s.Close()).ToNot(HaveOccurred())
			})

			It("query returns expected results", func() {
				type testCase struct {
					query       string
					segmentKeys []dimension.Key
				}

				testCases := []testCase{
					{`app.name`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{foo="bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
					}},
					{`app.name{foo=~"^b.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
					}},
					{`app.name{baz=~"xxx|qux"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{baz!="xxx"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo!="bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{foo!~".*"}`, []dimension.Key{
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{baz!~"^x.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo="bar",baz!~"^x.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},

					{`app.name{foo=~"b.*",foo!~".*r"}`, nil},

					{`app.name{foo!="non-existing-value"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{foo!~"non-existing-.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{non_existing_key!="bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{non_existing_key!~"bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},

					{`app.name{foo="non-existing-value"}`, nil},
					{`app.name{foo=~"non-existing-.*"}`, nil},
					{`app.name{non_existing_key="bar"}`, nil},
					{`app.name{non_existing_key=~"bar"}`, nil},

					{`non-existing-app{}`, nil},
				}

				for _, tc := range testCases {
					qry, err := flameql.ParseQuery(tc.query)
					Expect(err).ToNot(HaveOccurred())
					r := s.execQuery(context.TODO(), qry)
					if tc.segmentKeys == nil {
						Expect(r).To(BeEmpty())
						continue
					}
					Expect(r).To(ConsistOf(tc.segmentKeys))
				}
				Expect(s.Close()).ToNot(HaveOccurred())
			})
		})
	})
})

var _ = Describe("CollectGarbage", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry())
			Expect(err).ToNot(HaveOccurred())
		})

		Context("RetentionPolicy", func() {
			It("removes data outside retention period", func() {
				key, _ := segment.ParseKey("foo")
				tree := tree.New()
				tree.Insert([]byte("a;b"), uint64(1))
				tree.Insert([]byte("a;c"), uint64(2))
				now := time.Now()

				err := s.Put(&PutInput{
					StartTime:  now.Add(-3 * time.Hour),
					EndTime:    now.Add(-3 * time.Hour).Add(time.Second * 10),
					Key:        key,
					Val:        tree,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())

				err = s.Put(&PutInput{
					StartTime:  now.Add(-time.Minute),
					EndTime:    now.Add(-time.Minute).Add(time.Second * 10),
					Key:        key,
					Val:        tree,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())

				err = s.EnforceRetentionPolicy(segment.NewRetentionPolicy().SetAbsolutePeriod(time.Hour))
				Expect(err).ToNot(HaveOccurred())

				o, err := s.Get(&GetInput{
					StartTime: now.Add(-3 * time.Hour),
					EndTime:   now.Add(-3 * time.Hour).Add(time.Second * 10),
					Key:       key,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(o).To(BeNil())

				o, err = s.Get(&GetInput{
					StartTime: now.Add(-time.Minute),
					EndTime:   now.Add(-time.Minute).Add(time.Second * 10),
					Key:       key,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(o).ToNot(BeNil())

				Expect(s.Close()).ToNot(HaveOccurred())
			})
		})
	})
})

var _ = Describe("Getters", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry())
			Expect(err).ToNot(HaveOccurred())
		})

		It("gets app names correctly", func() {
			tree := tree.New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))
			st := testing.SimpleTime(10)
			et := testing.SimpleTime(19)
			key, _ := segment.ParseKey("foo")

			s.Put(&PutInput{
				StartTime:  st,
				EndTime:    et,
				Key:        key,
				Val:        tree,
				SpyName:    "testspy",
				SampleRate: 100,
			})

			want := []string{"foo"}
			Expect(s.GetAppNames()).To(Equal(
				want,
			))
			Expect(s.Close()).ToNot(HaveOccurred())
		})
	})
})
