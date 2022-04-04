//go:build !windows
// +build !windows

package storage

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("MergeProfiles", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
			Expect(err).ToNot(HaveOccurred())
		})
		Context("when profiles with ID ingested", func() {
			It("merges profiling data correctly", func() {
				defer s.Close()

				tree := tree.New()
				tree.Insert([]byte("a;b"), uint64(1))
				tree.Insert([]byte("a;c"), uint64(2))
				st := testing.SimpleTime(10)
				et := testing.SimpleTime(19)

				k1, _ := segment.ParseKey("app.cpu{profile_id=a}")
				Expect(s.Put(context.TODO(), &PutInput{
					StartTime: st,
					EndTime:   et,
					Key:       k1,
					Val:       tree,
				})).ToNot(HaveOccurred())

				Expect(s.Put(context.TODO(), &PutInput{
					StartTime: st,
					EndTime:   et,
					Key:       k1,
					Val:       tree,
				})).ToNot(HaveOccurred())

				k2, _ := segment.ParseKey("app.cpu{profile_id=b}")
				Expect(s.Put(context.TODO(), &PutInput{
					StartTime: st,
					EndTime:   et,
					Key:       k2,
					Val:       tree,
				})).ToNot(HaveOccurred())

				o, err := s.MergeProfiles(context.Background(), MergeProfilesInput{
					AppName:  "app.cpu",
					Profiles: []string{"a"},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(6)))

				o, err = s.MergeProfiles(context.Background(), MergeProfilesInput{
					AppName:  "app.cpu",
					Profiles: []string{"a", "b"},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(o.Tree).ToNot(BeNil())
				Expect(o.Tree.Samples()).To(Equal(uint64(9)))
			})
		})
	})
})

var _ = Describe("Profiles retention policy", func() {
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
					Val:       tree,
				})).ToNot(HaveOccurred())

				t3 := t2.Add(10 * time.Second)
				t4 := t3.Add(10 * time.Second)
				k2, _ := segment.ParseKey("app.cpu{profile_id=b}")
				Expect(s.Put(context.TODO(), &PutInput{
					StartTime: t3,
					EndTime:   t4,
					Key:       k2,
					Val:       tree,
				})).ToNot(HaveOccurred())

				rp := &segment.RetentionPolicy{ExemplarsRetentionTime: t3}
				Expect(s.EnforceRetentionPolicy(rp)).ToNot(HaveOccurred())

				o, err := s.MergeProfiles(context.Background(), MergeProfilesInput{
					AppName:  "app.cpu",
					Profiles: []string{"a", "b"},
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
