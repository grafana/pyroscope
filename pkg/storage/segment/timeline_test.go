package segment

import (
	"math/big"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("timeline", func() {
	var (
		timeline *Timeline
		st       int
		et       int
	)

	BeforeEach(func() {
		st = 0
		et = 40
	})
	JustBeforeEach(func() {
		timeline = GenerateTimeline(
			testing.SimpleTime(st),
			testing.SimpleTime(et),
		)
	})

	Describe("PopulateTimeline", func() {
		Context("empty segment", func() {
			It("works as expected", func(done Done) {
				s := New()
				timeline.PopulateTimeline(s, new(Threshold))
				Expect(timeline.Samples).To(Equal([]uint64{
					0,
					0,
					0,
					0,
				}))
				close(done)
			})
		})
		Context("one level", func() {
			It("works as expected", func(done Done) {
				s := New()
				s.Put(testing.SimpleTime(0),
					testing.SimpleTime(9), 2, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 5, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleTime(20),
					testing.SimpleTime(29), 0, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				timeline.PopulateTimeline(s, new(Threshold))
				Expect(timeline.Samples).To(Equal([]uint64{
					3,
					6,
					1,
					0,
				}))

				close(done)
			}, 5)
		})
		Context("multiple levels", func() {
			BeforeEach(func() {
				st = 0
				et = 365 * 24 * 60 * 60
			})

			It("works as expected", func(done Done) {
				s := New()
				s.Put(testing.SimpleTime(0),
					testing.SimpleTime(9), 2, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 5, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleTime(20),
					testing.SimpleTime(29), 0, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				timeline.PopulateTimeline(s, new(Threshold))
				expected := make([]uint64, 3153)
				expected[0] = 8
				Expect(timeline.Samples).To(Equal(expected))

				close(done)
			}, 5)
		})

		Context("with threshold", func() {
			BeforeEach(func() {
				st = 0
				et = 365 * 24 * 60 * 60
			})

			It("removed nodes are down-sampled", func(done Done) {
				s := New()
				now := time.Now()
				s.Put(testing.SimpleTime(0),
					testing.SimpleTime(9), 2, func(de int, t time.Time, r *big.Rat, a []Addon) {})
				s.Put(testing.SimpleTime(10),
					testing.SimpleTime(19), 5, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				// To prevent segment root removal.
				s.Put(now.Add(-10*time.Second),
					now, 0, func(de int, t time.Time, r *big.Rat, a []Addon) {})

				threshold := NewThreshold().
					SetLevelMaxAge(0, time.Second).
					SetLevelMaxAge(1, time.Minute)

				s.DeleteDataBefore(threshold, func(int, time.Time) {})
				timeline.PopulateTimeline(s, threshold)
				expected := make([]uint64, 3153)
				expected[0] = 8
				Expect(timeline.Samples).To(Equal(expected))

				close(done)
			}, 5)
		})
	})
})
