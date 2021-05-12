package bytesize

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("bytesize package", func() {
	Describe("Parse", func() {
		It("works with valid values", func() {
			Expect(Parse("1TB")).To(Equal(1 * TB))
			Expect(Parse("1 TB")).To(Equal(1 * TB))
			Expect(Parse(" 1 TB ")).To(Equal(1 * TB))
			Expect(Parse("  1  TB  ")).To(Equal(1 * TB))

			Expect(Parse("1.0TB")).To(BeNumerically("~", 1*TB, GB))
			Expect(Parse("1.9TB")).To(BeNumerically("~", 1*TB+921*GB, GB))

			Expect(Parse("1")).To(Equal(1 * Byte))
			Expect(Parse(" 1 ")).To(Equal(1 * Byte))

			Expect(Parse("1mb")).To(Equal(1 * MB))
			Expect(Parse("1mB")).To(Equal(1 * MB))
		})
		It("returns error with invalid values", func() {
			_, err := Parse("1UB")
			Expect(err).To(MatchError("could not parse ByteSize"))
		})
	})
})
