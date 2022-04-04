//go:build !windows
// +build !windows

package storage

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("storage package", func() {
	var s *Storage

	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("delete app", func() {
		/*************************************/
		/*  h e l p e r   f u n c t i o n s  */
		/*************************************/
		checkSegmentsPresence := func(appname string, presence bool) {
			segmentKey, err := segment.ParseKey(string(appname))
			Expect(err).ToNot(HaveOccurred())
			segmentKeyStr := segmentKey.SegmentKey()
			Expect(segmentKeyStr).To(Equal(appname + "{}"))
			_, ok := s.segments.Cache.Lookup(segmentKeyStr)

			if presence {
				Expect(ok).To(BeTrue())
			} else {
				Expect(ok).To(BeFalse())
			}
		}

		checkDimensionsPresence := func(appname string, presence bool) interface{} {
			d, ok := s.lookupAppDimension(appname)
			if presence {
				Expect(ok).To(BeTrue())
			} else {
				Expect(ok).To(BeFalse())
			}

			return d
		}

		checkTreesPresence := func(appname string, st time.Time, depth int, presence bool) interface{} {
			key, err := segment.ParseKey(appname)
			Expect(err).ToNot(HaveOccurred())
			treeKeyName := key.TreeKey(depth, st)
			t, ok := s.trees.Cache.Lookup(treeKeyName)
			if presence {
				Expect(ok).To(BeTrue())
			} else {
				Expect(ok).To(BeFalse())
			}

			return t
		}

		checkDictsPresence := func(appname string, presence bool) interface{} {
			d, ok := s.dicts.Cache.Lookup(appname)
			if presence {
				Expect(ok).To(BeTrue())
			} else {
				Expect(ok).To(BeFalse())
			}
			return d
		}

		checkLabelsPresence := func(appname string, presence bool) {
			// this indirectly calls s.labels
			appnames := s.GetAppNames(context.TODO())

			// linear scan should be fast enough here
			found := false
			for _, v := range appnames {
				if v == appname {
					found = true
				}
			}

			if presence {
				Expect(found).To(BeTrue())
			} else {
				Expect(found).To(BeFalse())
			}
		}

		Context("simple app", func() {
			It("works correctly", func() {
				appname := "my.app.cpu"

				// We insert info for an app
				tree1 := tree.New()
				tree1.Insert([]byte("a;b"), uint64(1))

				st := testing.SimpleTime(10)
				et := testing.SimpleTime(19)
				key, _ := segment.ParseKey(appname)
				err := s.Put(context.TODO(), &PutInput{
					StartTime:  st,
					EndTime:    et,
					Key:        key,
					Val:        tree1,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())

				// Since the DeleteApp also removes dictionaries
				// therefore we need to create them manually here
				// (they are normally created when TODO)
				d := dict.New()
				s.dicts.Put(appname, d)

				/*******************************/
				/*  S a n i t y   C h e c k s  */
				/*******************************/
				// Dimensions
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(1)))
				checkDimensionsPresence(appname, true)

				// Trees
				Expect(s.trees.Cache.Size()).To(Equal(uint64(1)))
				checkTreesPresence(appname, st, 0, true)

				// Segments
				Expect(s.segments.Cache.Size()).To(Equal(uint64(1)))
				checkSegmentsPresence(appname, true)

				// Dicts
				// I manually inserted a dictionary so it should be fine?
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(1)))
				checkDictsPresence(appname, true)

				// Labels
				checkLabelsPresence(appname, true)

				/*************************/
				/*  D e l e t e   a p p  */
				/*************************/
				err = s.DeleteApp(context.TODO(), appname)
				Expect(err).ToNot(HaveOccurred())

				// Trees
				// should've been deleted from CACHE
				Expect(s.trees.Cache.Size()).To(Equal(uint64(0)))
				checkTreesPresence(appname, st, 0, false)

				// Dimensions
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(0)))
				checkDimensionsPresence(appname, false)

				// Dicts
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(0)))
				checkDictsPresence(appname, false)

				// Segments
				Expect(s.segments.Cache.Size()).To(Equal(uint64(0)))
				checkSegmentsPresence(appname, false)

				// Labels
				checkLabelsPresence(appname, false)
			})
		})

		Context("app with labels", func() {
			It("works correctly", func() {
				appname := "my.app.cpu"

				// We insert info for an app
				tree1 := tree.New()
				tree1.Insert([]byte("a;b"), uint64(1))

				// We are mirroring this on the simple.golang.cpu example
				labels := []string{
					"",
					"{foo=bar,function=fast}",
					"{foo=bar,function=slow}",
				}

				st := testing.SimpleTime(10)
				et := testing.SimpleTime(19)
				for _, l := range labels {
					key, _ := segment.ParseKey(appname + l)
					err := s.Put(context.TODO(), &PutInput{
						StartTime:  st,
						EndTime:    et,
						Key:        key,
						Val:        tree1,
						SpyName:    "testspy",
						SampleRate: 100,
					})
					Expect(err).ToNot(HaveOccurred())
				}

				// Since the DeleteApp also removes dictionaries
				// therefore we need to create them manually here
				// (they are normally created when TODO)
				d := dict.New()
				s.dicts.Put(appname, d)

				/*******************************/
				/*  S a n i t y   C h e c k s  */
				/*******************************/

				By("checking dimensions were created")
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(4)))
				checkDimensionsPresence(appname, true)

				By("checking trees were created")
				Expect(s.trees.Cache.Size()).To(Equal(uint64(3)))
				checkTreesPresence(appname, st, 0, true)

				By("checking segments were created")
				Expect(s.segments.Cache.Size()).To(Equal(uint64(3)))
				checkSegmentsPresence(appname, true)

				By("checking dicts were created")
				// Dicts
				// I manually inserted a dictionary so it should be fine?
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(1)))
				checkDictsPresence(appname, true)

				// Labels
				checkLabelsPresence(appname, true)

				/*************************/
				/*  D e l e t e   a p p  */
				/*************************/
				By("deleting the app")
				err := s.DeleteApp(context.TODO(), appname)
				Expect(err).ToNot(HaveOccurred())

				By("checking trees were deleted")
				// Trees
				// should've been deleted from CACHE
				Expect(s.trees.Cache.Size()).To(Equal(uint64(0)))
				checkTreesPresence(appname, st, 0, false)

				// Dimensions
				By("checking dimensions were deleted")
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(0)))
				checkDimensionsPresence(appname, false)

				// Dicts
				By("checking dicts were deleted")
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(0)))
				checkDictsPresence(appname, false)

				// Segments
				By("checking segments were deleted")
				Expect(s.segments.Cache.Size()).To(Equal(uint64(0)))
				checkSegmentsPresence(appname, false)

				// Labels
				By("checking labels were deleted")
				checkLabelsPresence(appname, false)
			})
		})

		// In this test we have 2 apps with the same label
		// And deleting one app should not interfer with the labels of the other app
		Context("multiple apps with labels", func() {
			It("deletes the correct data", func() {
				st := testing.SimpleTime(10)
				insert := func(appname string) {
					// We insert info for an app
					tree1 := tree.New()
					tree1.Insert([]byte("a;b"), uint64(1))

					// We are mirroring this on the simple.golang.cpu example
					labels := []string{
						"",
						"{foo=bar,function=fast}",
						"{foo=bar,function=slow}",
					}

					et := testing.SimpleTime(19)
					for _, l := range labels {
						key, _ := segment.ParseKey(appname + l)
						err := s.Put(context.TODO(), &PutInput{
							StartTime:  st,
							EndTime:    et,
							Key:        key,
							Val:        tree1,
							SpyName:    "testspy",
							SampleRate: 100,
						})
						Expect(err).ToNot(HaveOccurred())
					}

					// Since the DeleteApp also removes dictionaries
					// therefore we need to create them manually here
					// (they are normally created when TODO)
					d := dict.New()
					s.dicts.Put(appname, d)
				}

				app1name := "myapp1.cpu"
				app2name := "myapp2.cpu"

				insert(app1name)
				insert(app2name)

				/*******************************/
				/*  S a n i t y   C h e c k s  */
				/*******************************/
				By("checking dimensions were created")
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(5)))
				checkDimensionsPresence(app1name, true)
				checkDimensionsPresence(app2name, true)

				By("checking trees were created")
				Expect(s.trees.Cache.Size()).To(Equal(uint64(6)))
				checkTreesPresence(app1name, st, 0, true)
				checkTreesPresence(app2name, st, 0, true)

				By("checking segments were created")
				Expect(s.segments.Cache.Size()).To(Equal(uint64(6)))
				checkSegmentsPresence(app1name, true)
				checkSegmentsPresence(app2name, true)

				By("checking dicts were created")
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(2)))
				checkDictsPresence(app1name, true)
				checkDictsPresence(app2name, true)

				By("checking labels were created")
				checkLabelsPresence(app1name, true)
				checkLabelsPresence(app2name, true)

				/*************************/
				/*  D e l e t e   a p p  */
				/*************************/
				By("deleting the app")
				err := s.DeleteApp(context.TODO(), app1name)
				Expect(err).ToNot(HaveOccurred())

				By("checking trees were deleted")
				Expect(s.trees.Cache.Size()).To(Equal(uint64(3)))
				checkTreesPresence(app1name, st, 0, false)
				checkTreesPresence(app2name, st, 0, true)

				// Dimensions
				By("checking dimensions were deleted")
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(4)))

				// Dimensions that refer to app2 are still intact
				v, ok := s.dimensions.Lookup("__name__:myapp2.cpu")
				Expect(ok).To(Equal(true))
				Expect(v.(*dimension.Dimension).Keys).To(Equal([]dimension.Key{
					dimension.Key("myapp2.cpu{foo=bar,function=fast}"),
					dimension.Key("myapp2.cpu{foo=bar,function=slow}"),
					dimension.Key("myapp2.cpu{}"),
				}))

				v, ok = s.dimensions.Lookup("foo:bar")
				Expect(ok).To(Equal(true))
				Expect(v.(*dimension.Dimension).Keys).To(Equal([]dimension.Key{
					dimension.Key("myapp2.cpu{foo=bar,function=fast}"),
					dimension.Key("myapp2.cpu{foo=bar,function=slow}"),
				}))

				v, ok = s.dimensions.Lookup("function:fast")
				Expect(ok).To(Equal(true))
				Expect(v.(*dimension.Dimension).Keys).To(Equal([]dimension.Key{
					dimension.Key("myapp2.cpu{foo=bar,function=fast}"),
				}))

				v, ok = s.dimensions.Lookup("function:slow")
				Expect(ok).To(Equal(true))
				Expect(v.(*dimension.Dimension).Keys).To(Equal([]dimension.Key{
					dimension.Key("myapp2.cpu{foo=bar,function=slow}"),
				}))

				By("checking dicts were deleted")
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(1)))
				checkDictsPresence(app1name, false)
				checkDictsPresence(app2name, true)

				By("checking segments were deleted")
				Expect(s.segments.Cache.Size()).To(Equal(uint64(3)))
				checkSegmentsPresence(app1name, false)
				checkSegmentsPresence(app2name, true)

				By("checking labels were deleted")
				checkLabelsPresence(app1name, false)
				checkSegmentsPresence(app2name, true)
			})
		})

		// Delete an unrelated app
		// It should not fail
		It("is idempotent", func() {
			appname := "my.app.cpu"

			// We insert info for an app
			tree1 := tree.New()
			tree1.Insert([]byte("a;b"), uint64(1))

			// We are mirroring this on the simple.golang.cpu example
			labels := []string{
				"",
				"{foo=bar,function=fast}",
				"{foo=bar,function=slow}",
			}

			st := testing.SimpleTime(10)
			et := testing.SimpleTime(19)
			for _, l := range labels {
				key, _ := segment.ParseKey(appname + l)
				err := s.Put(context.TODO(), &PutInput{
					StartTime:  st,
					EndTime:    et,
					Key:        key,
					Val:        tree1,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())
			}

			// Since the DeleteApp also removes dictionaries
			// therefore we need to create them manually here
			// (they are normally created when TODO)
			d := dict.New()
			s.dicts.Put(appname, d)

			/*******************************/
			/*  S a n i t y   C h e c k s  */
			/*******************************/
			sanityChecks := func() {
				By("checking dimensions were created")
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(4)))
				checkDimensionsPresence(appname, true)

				By("checking trees were created")
				Expect(s.trees.Cache.Size()).To(Equal(uint64(3)))
				checkTreesPresence(appname, st, 0, true)

				By("checking segments were created")
				Expect(s.segments.Cache.Size()).To(Equal(uint64(3)))
				checkSegmentsPresence(appname, true)

				By("checking dicts were created")
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(1)))
				checkDictsPresence(appname, true)

				checkLabelsPresence(appname, true)
			}

			sanityChecks()

			/*************************/
			/*  D e l e t e   a p p  */
			/*************************/
			By("deleting the app")
			err := s.DeleteApp(context.TODO(), "random.app")
			Expect(err).ToNot(HaveOccurred())

			// nothing should have happened
			// since the deleted app does not exist
			sanityChecks()
		})
	})
})
