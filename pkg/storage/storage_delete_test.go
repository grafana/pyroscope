package storage

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("storage package", func() {
	var s *Storage

	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("delete app", func() {
		Context("simple app", func() {
			It("works correctly", func() {
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

				checkDimensionsPresence := func(appname string, presence bool) {
					_, ok := s.lookupAppDimension(appname)
					if presence {
						Expect(ok).To(BeTrue())
					} else {
						Expect(ok).To(BeFalse())
					}
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
				t := checkTreesPresence(appname, st, 0, true)
				Expect(t).To(Equal(tree1))

				// Segments
				Expect(s.segments.Cache.Size()).To(Equal(uint64(1)))
				checkSegmentsPresence(appname, true)

				// Dicts
				// I manually inserted a dictionary so it should be fine?
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(1)))
				checkDictsPresence(appname, true)

				/*************************/
				/*  D e l e t e   a p p  */
				/*************************/
				err = s.DeleteApp(appname)
				Expect(err).ToNot(HaveOccurred())

				// Trees
				// should've been deleted from CACHE
				Expect(s.trees.Cache.Size()).To(Equal(uint64(0)))
				t = checkTreesPresence(appname, st, 0, false)
				// Trees should've been also deleted from DISK
				// TODO: how to check for that?

				// Dimensions
				Expect(s.dimensions.Cache.Size()).To(Equal(uint64(0)))
				checkDimensionsPresence(appname, false)

				// Dicts
				Expect(s.dicts.Cache.Size()).To(Equal(uint64(0)))
				checkDictsPresence(appname, false)

				// Segments
				Expect(s.segments.Cache.Size()).To(Equal(uint64(0)))
				checkSegmentsPresence(appname, false)
			})

		})
	})
})
