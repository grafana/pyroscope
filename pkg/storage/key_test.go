package storage

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("storage package", func() {
	Context("ParseKey", func() {
		It("simple values work", func() {
			k, err := ParseKey("foo{bar=1,baz=2}")
			Expect(err).ToNot(HaveOccurred())
			Expect(k.labels).To(Equal(map[string]string{"__name__": "foo", "bar": "1", "baz": "2"}))
		})

		It("simple values with spaces work", func() {
			k, err := ParseKey(" foo { bar = 1 , baz = 2 } ")
			Expect(err).ToNot(HaveOccurred())
			Expect(k.labels).To(Equal(map[string]string{"__name__": "foo", "bar": "1", "baz": "2"}))
		})
	})

	Context("Key", func() {
		Context("Normalize", func() {
			It("simple values work", func() {
				k, err := ParseKey("foo{bar=1,baz=2}")
				Expect(err).ToNot(HaveOccurred())
				Expect(k.Normalized()).To(Equal("foo{bar=1,baz=2}"))
			})

			It("unsorted values work", func() {
				k, err := ParseKey("foo{baz=1,bar=2}")
				Expect(err).ToNot(HaveOccurred())
				Expect(k.Normalized()).To(Equal("foo{bar=2,baz=1}"))
			})
		})
	})
})
