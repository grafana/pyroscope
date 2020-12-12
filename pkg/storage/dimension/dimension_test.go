package dimension

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("dimension", func() {
	Context("Intersection", func() {
		It("works", func() {
			d1 := New()
			d1.Insert(key("bar"))
			d1.Insert(key("baz"))
			d1.Insert(key("foo"))

			d2 := New()
			d2.Insert(key("foo"))
			d2.Insert(key("baz"))
			d2.Insert(key("bar"))

			d3 := New()
			d3.Insert(key("bar"))

			d4 := New()

			Expect(Intersection(d1, d2, d3, d4)).To(Equal([]key{}))
			Expect(Intersection(d1, d2, d3)).To(Equal([]key{key("bar")}))
			Expect(Intersection(d1, d2)).To(Equal([]key{
				key("bar"),
				key("baz"),
				key("foo"),
			}))
		})
	})
})
