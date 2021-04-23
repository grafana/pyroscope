package segment

import (
	"log"
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

func (sm *storageMock) Get(st, et time.Time, cb func(depth int, samples, writes uint64, t time.Time, r *big.Rat)) {
	st, et = normalize(st, et)
	for _, d := range sm.data {
		if !d.t.Before(st) && !d.t.Add(sm.resolution).After(et) {
			cb(0, 1, 1, d.t, d.r)
		}
	}
}

// if you change something in this test make sure it doesn't change test coverage.
func fuzzTest(writeSize func() int) {
	s := New(10*time.Second, 10)
	m := newMock(10 * time.Second)

	r := rand.New(rand.NewSource(1213))

	for k := 0; k < 20; k++ {
		maxStartTime := r.Intn(5000)
		// for i := 0; i < 10; i++ {
		for i := 0; i < r.Intn(200); i++ {
			sti := r.Intn(maxStartTime) * 10
			st := testing.SimpleTime(sti)
			et := testing.SimpleTime(sti + writeSize())
			dur := et.Sub(st)

			// samples := uint64(1+r.Intn(10)) * uint64(dur/(10*time.Second))
			samples := uint64(20)

			m.Put(st, et, samples)
			s.Put(st, et, samples, func(depth int, t time.Time, r *big.Rat, addons []Addon) {
				log.Println(depth, r, dur)
			})
		}
		mSum := big.NewRat(0, 1)
		mWrites := big.NewRat(0, 1)
		sSum := big.NewRat(0, 1)
		sWrites := big.NewRat(0, 1)
		for i := 0; i < r.Intn(100); i++ {
			sti := r.Intn(100) * 10
			st := testing.SimpleTime(sti)
			et := testing.SimpleTime(sti + r.Intn(100)*10)

			m.Get(st, et, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
				rClone := big.NewRat(r.Num().Int64(), r.Denom().Int64())
				mSum.Add(mSum, rClone.Mul(rClone, big.NewRat(int64(samples), 1)))
				log.Println("mWrites", samples, writes, r)
				// if r.Num().Int64() > 0 {
				// r = r.Inv(r)
				w := big.NewRat(int64(writes), 1)
				// mWrites.Add(mWrites, r.Mul(r, w))
				mWrites.Add(mWrites, w)
				// }
			})

			s.Get(st, et, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
				rClone := big.NewRat(r.Num().Int64(), r.Denom().Int64())
				sSum.Add(sSum, rClone.Mul(rClone, big.NewRat(int64(samples), 1)))
				log.Println("sWrites", samples, writes, r)
				// if r.Num().Int64() > 0 {
				// r = r.Inv(r)
				w := big.NewRat(int64(writes), 1)
				// sWrites.Add(sWrites, r.Mul(r, w))
				sWrites.Add(sWrites, w)
				// }
			})
		}
		mSumF, _ := mSum.Float64()
		mWritesF, _ := mWrites.Float64()
		log.Println("m:", mSum, mSumF, mWrites, mWritesF)

		sSumF, _ := sSum.Float64()
		sWritesF, _ := sWrites.Float64()
		log.Println("s:", sSum, sSumF, sWrites, sWritesF)

		Expect(mSum.Cmp(sSum)).To(Equal(0))
		Expect(mWrites.Cmp(sWrites)).To(Equal(0))
	}
}

// See https://github.com/pyroscope-io/pyroscope/issues/28 for more context
var _ = Describe("segment", func() {
	Context("fuzz tests", func() {
		Context("writes are 10 second long", func() {
			It("works as expected", func(done Done) {
				fuzzTest(func() int {
					return 10
				})
				close(done)
			}, 5)
		})
		Context("writes are different lengths", func() {
			// It("works as expected", func(done Done) {
			// 	fuzzTest(func() int {
			// 		return 20
			// 		// return 1 + rand.Intn(10)*10
			// 	})
			// 	close(done)
			// }, 5)
		})
	})
})
