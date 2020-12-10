package segment

import (
	"math/big"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/petethepig/pyroscope/pkg/testing"
)

// 	inside  rel = iota // | S E |            <1
// 	match              // matching ranges    1/1
// 	outside            // | | S E            0/1
// 	overlap            // | S | E            <1
// 	contain            // S | | E            1/1

var _ = Describe("stree", func() {
	FContext("overlap", func() {
		Context("match", func() {
			It("returns correct values", func() {
				Expect(overlapAmount(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(0), testing.SimpleTime(100), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(1, 1).String()))
			})
		})
		Context("inside", func() {
			It("returns correct values", func() {
				Expect(overlapAmount(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(10), testing.SimpleTime(90), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(8, 10).String()))
				Expect(overlapAmount(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(0), testing.SimpleTime(90), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(9, 10).String()))
				Expect(overlapAmount(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(10), testing.SimpleTime(100), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(9, 10).String()))
			})
		})
		Context("contain", func() {
			It("returns correct values", func() {
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(210), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(1, 1).String()))
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(100), testing.SimpleTime(210), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(1, 1).String()))
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(200), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(1, 1).String()))
			})
		})
		Context("overlap", func() {
			It("returns correct values", func() {
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(110), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(1, 10).String()))
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(190), testing.SimpleTime(210), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(1, 10).String()))
			})
		})
		Context("outside", func() {
			It("returns correct values", func() {
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(100), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(0, 1).String()))
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(80), testing.SimpleTime(90), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(0, 1).String()))
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(200), testing.SimpleTime(210), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(0, 1).String()))
				Expect(overlapAmount(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(210), testing.SimpleTime(220), // st et
					10*time.Second,
				).String()).To(Equal(big.NewRat(0, 1).String()))
			})
		})
	})
})
