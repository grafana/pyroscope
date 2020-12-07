package segment

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/petethepig/pyroscope/pkg/testing"
)

// 	inside  rel = iota // | S E |
// 	match              // matching ranges
// 	outside            // | | S E
// 	overlap            // | S | E
// 	contain            // S | | E

var _ = Describe("stree", func() {
	Context("relationship", func() {
		Context("match", func() {
			It("returns correct values", func() {
				Expect(relationship(
					testing.SimpleTime(0), testing.SimpleTime(10), // t1 t2
					testing.SimpleTime(0), testing.SimpleTime(10), // st et
				).String()).To(Equal("match"))
			})
		})
		Context("inside", func() {
			Expect(relationship(
				testing.SimpleTime(0), testing.SimpleTime(10), // t1 t2
				testing.SimpleTime(1), testing.SimpleTime(9), // st et
			).String()).To(Equal("inside"))
			Expect(relationship(
				testing.SimpleTime(0), testing.SimpleTime(10), // t1 t2
				testing.SimpleTime(0), testing.SimpleTime(9), // st et
			).String()).To(Equal("inside"))
			Expect(relationship(
				testing.SimpleTime(0), testing.SimpleTime(10), // t1 t2
				testing.SimpleTime(1), testing.SimpleTime(10), // st et
			).String()).To(Equal("inside"))
		})
		Context("contain", func() {
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(9), testing.SimpleTime(21), // st et
			).String()).To(Equal("contain"))
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(10), testing.SimpleTime(21), // st et
			).String()).To(Equal("contain"))
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(9), testing.SimpleTime(20), // st et
			).String()).To(Equal("contain"))
		})
		Context("overlap", func() {
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(9), testing.SimpleTime(11), // st et
			).String()).To(Equal("overlap"))
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(19), testing.SimpleTime(21), // st et
			).String()).To(Equal("overlap"))
		})
		Context("outside", func() {
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(9), testing.SimpleTime(10), // st et
			).String()).To(Equal("outside"))
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(8), testing.SimpleTime(9), // st et
			).String()).To(Equal("outside"))
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(20), testing.SimpleTime(21), // st et
			).String()).To(Equal("outside"))
			Expect(relationship(
				testing.SimpleTime(10), testing.SimpleTime(20), // t1 t2
				testing.SimpleTime(21), testing.SimpleTime(22), // st et
			).String()).To(Equal("outside"))
		})
	})
})
