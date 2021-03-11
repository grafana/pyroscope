package segment

import (
	"math/big"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

type datapoint struct {
	t       time.Time
	samples uint64
	r       *big.Rat
}

type storageMock struct {
	resolution time.Duration
	data       []datapoint
}

func newMock(resolution time.Duration) *storageMock {
	return &storageMock{
		resolution: resolution,
		data:       []datapoint{},
	}
}

func (sm *storageMock) Put(st, et time.Time, samples uint64) {
	st, et = normalize(st, et)
	fullDur := et.Sub(st) / sm.resolution
	for t := st; t.Before(et); t = t.Add(sm.resolution) {
		d := datapoint{
			t:       t,
			samples: samples,
			r:       big.NewRat(int64(samples), int64(fullDur)),
		}

		sm.data = append(sm.data, d)
	}
}

func (sm *storageMock) Get(st, et time.Time, cb func(depth int, samples uint64, t time.Time, r *big.Rat)) {
	st, et = normalize(st, et)
	for _, d := range sm.data {
		if !d.t.Before(st) && !d.t.Add(sm.resolution).After(et) {
			cb(0, 1, d.t, d.r)
		}
	}
}

var _ = Describe("segment", func() {
	Context("Segment", func() {
		It("works", func(done Done) {
			s := New(10*time.Second, 10)
			m := newMock(10 * time.Second)

			r := rand.New(rand.NewSource(123))

			for k := 0; k < 100; k++ {
				for i := 0; i < r.Intn(1000); i++ {
					sti := r.Intn(100) * 10
					st := testing.SimpleTime(sti)
					et := testing.SimpleTime(sti + 10)
					samples := uint64(r.Intn(100))

					m.Put(st, et, samples)
					s.Put(st, et, samples, func(depth int, t time.Time, r *big.Rat, addons []Addon) {

					})
				}
				mSum := big.NewRat(0, 1)
				sSum := big.NewRat(0, 1)
				for i := 0; i < r.Intn(100); i++ {
					sti := r.Intn(100) * 10
					st := testing.SimpleTime(sti)
					et := testing.SimpleTime(sti + 160)
					// et := testing.SimpleTime(sti + r.Intn(100)*10)

					m.Get(st, et, func(depth int, samples uint64, t time.Time, r *big.Rat) {
						mSum.Add(mSum, r.Mul(r, big.NewRat(int64(samples), 1)))
					})

					s.Get(st, et, func(depth int, samples uint64, t time.Time, r *big.Rat) {
						sSum.Add(sSum, r.Mul(r, big.NewRat(int64(samples), 1)))
					})
				}
				Expect(mSum.Cmp(sSum)).To(Equal(0))
			}
			close(done)
		}, 5)
	})
})
