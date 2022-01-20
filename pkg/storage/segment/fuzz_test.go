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
func fuzzTest(testWrites bool, writeSize func() int) {
	s := New()
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
		if testWrites {
			Expect(mWrites.Cmp(sWrites)).To(Equal(0))
		}
	}
}

// See https://github.com/pyroscope-io/pyroscope/issues/28 for more context
var _ = Describe("segment", func() {
	Context("fuzz tests", func() {
		Context("writes are 10 second long", func() {
			It("works as expected", func(done Done) {
				fuzzTest(true, func() int {
					return 10
				})
				close(done)
			}, 5)
		})
		Context("writes are different lengths", func() {
			It("works as expected", func(done Done) {
				fuzzTest(false, func() int {
					return 20
					// return 1 + rand.Intn(10)*10
				})
				close(done)
			}, 5)
		})
	})

	Context("retention and sampling randomized test", func() {
		It("works as expected", func() {
			s := New()
			r := rand.New(rand.NewSource(7332))
			w := testSegWriter{
				n: 10e4,
				r: r,

				samplesPerWrite:  100,
				writeTimeSpanSec: 10,
				startTimeMin:     randInt(1000, 3000),
				startTimeMax:     randInt(7000, 100000),

				buckets: make([]*bucket, 10),
			}

			w.write(s)

			for _, b := range w.buckets {
				removed, err := s.DeleteNodesBefore(&RetentionPolicy{AbsoluteTime: b.time})
				Expect(err).ToNot(HaveOccurred())
				// Actually, it is not pre-determined.
				Expect(removed).To(BeFalse())
				samples, writes := totalSamplesWrites(s, time.Time{}, testing.SimpleTime(w.startTimeMax*10))
				Expect(samples).To(Equal(b.samples))
				Expect(writes).To(Equal(b.writes))
			}

			st := testing.SimpleTime(w.startTimeMax * 10)
			samples, writes := totalSamplesWrites(s, st, st.Add(time.Hour))
			Expect(samples).To(BeZero())
			Expect(writes).To(BeZero())
		})
	})
})

type testSegWriter struct {
	r *rand.Rand
	n int

	samplesPerWrite  int
	writeTimeSpanSec int
	expectedWrites   int

	startTimeMin int
	startTimeMax int

	buckets []*bucket
}

type bucket struct {
	time    time.Time
	samples int
	writes  int
}

func (f testSegWriter) putStartEndTime() (st time.Time, et time.Time) {
	st = testing.SimpleTime(randInt(f.startTimeMin, f.startTimeMax) * 10)
	et = st.Add(time.Second * time.Duration(f.writeTimeSpanSec))
	return st, et
}

func randInt(min, max int) int { return rand.Intn(max-min) + min }

func (f testSegWriter) expectedSamples() int { return f.n * f.samplesPerWrite }

func (f testSegWriter) write(s *Segment) {
	// Initialize time buckets, if required.
	if len(f.buckets) > 0 {
		step := (f.startTimeMax - f.startTimeMin) / len(f.buckets) * 10
		for i := 0; i < len(f.buckets); i++ {
			f.buckets[i] = &bucket{time: testing.SimpleTime(f.startTimeMin + step*i)}
		}
	}
	cb := func(depth int, t time.Time, r *big.Rat, addons []Addon) {}
	for i := 0; i < f.n; i++ {
		st, et := f.putStartEndTime()
		err := s.Put(st, et, uint64(f.samplesPerWrite), cb)
		Expect(err).ToNot(HaveOccurred())
		for _, b := range f.buckets {
			if et.After(b.time) {
				b.samples += f.samplesPerWrite
				b.writes++
			}
		}
	}
}

func totalSamplesWrites(s *Segment, st, et time.Time) (samples, writes int) {
	v := big.NewRat(0, 1)
	s.Get(st, et, func(depth int, s, w uint64, t time.Time, r *big.Rat) {
		x := big.NewRat(r.Num().Int64(), r.Denom().Int64())
		v.Add(v, x.Mul(x, big.NewRat(int64(s), 1)))
		writes += int(w)
	})
	return int(v.Num().Int64()), writes
}
