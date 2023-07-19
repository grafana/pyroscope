package segment

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/grafana/pyroscope/pkg/og/testing"
)

//  relationship                               overlap read             overlap write
// 	inside  rel = iota   // | S E |            <1                       1/1
// 	match                // matching ranges    1/1                      1/1
// 	outside              // | | S E            0/1                      0/1
// 	overlap              // | S | E            <1                       <1
// 	contain              // S | | E            1/1                      <1

var _ = Describe("stree", func() {
	Context("relationship", func() {
		Context("match", func() {
			It("returns correct values", func() {
				Expect(relationship(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(0), testing.SimpleTime(100), // st et
				).String()).To(Equal("match"))
			})
		})
		Context("inside", func() {
			It("returns correct values", func() {
				Expect(relationship(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(10), testing.SimpleTime(90), // st et
				).String()).To(Equal("inside"))
				Expect(relationship(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(0), testing.SimpleTime(90), // st et
				).String()).To(Equal("inside"))
				Expect(relationship(
					testing.SimpleTime(0), testing.SimpleTime(100), // t1 t2
					testing.SimpleTime(10), testing.SimpleTime(100), // st et
				).String()).To(Equal("inside"))
			})
		})
		Context("contain", func() {
			It("returns correct values", func() {
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(210), // st et
				).String()).To(Equal("contain"))
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(100), testing.SimpleTime(210), // st et
				).String()).To(Equal("contain"))
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(200), // st et
				).String()).To(Equal("contain"))
			})
		})
		Context("overlap", func() {
			It("returns correct values", func() {
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(110), // st et
				).String()).To(Equal("overlap"))
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(190), testing.SimpleTime(210), // st et
				).String()).To(Equal("overlap"))
			})
		})
		Context("outside", func() {
			It("returns correct values", func() {
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(90), testing.SimpleTime(100), // st et
				).String()).To(Equal("outside"))
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(80), testing.SimpleTime(90), // st et
				).String()).To(Equal("outside"))
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(200), testing.SimpleTime(210), // st et
				).String()).To(Equal("outside"))
				Expect(relationship(
					testing.SimpleTime(100), testing.SimpleTime(200), // t1 t2
					testing.SimpleTime(210), testing.SimpleTime(220), // st et
				).String()).To(Equal("outside"))
			})
		})
	})
})
