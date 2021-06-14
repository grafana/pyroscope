package dimension

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("dimension", func() {
	Context("Intersection", func() {
		It("works", func() {
			d1 := New()
			d1.Insert(Key("bar"))
			d1.Insert(Key("baz"))
			d1.Insert(Key("foo"))

			d2 := New()
			d2.Insert(Key("foo"))
			d2.Insert(Key("baz"))
			d2.Insert(Key("bar"))

			d3 := New()
			d3.Insert(Key("bar"))

			d4 := New()

			Expect(Intersection(d1, d2, d3, d4)).To(Equal([]Key{}))
			Expect(Intersection(d1, d2, d3)).To(Equal([]Key{Key("bar")}))
			Expect(Intersection(d1, d2)).To(Equal([]Key{
				Key("bar"),
				Key("baz"),
				Key("foo"),
			}))
		})
	})

	Context("Union", func() {
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

			Expect(Union(d1, d2, d3, d4)).To(ConsistOf([]key{
				key("bar"),
				key("baz"),
				key("foo"),
			}))
			Expect(Union(d1, d2, d3)).To(ConsistOf([]key{
				key("bar"),
				key("baz"),
				key("foo"),
			}))
			Expect(Union(d3, d4)).To(ConsistOf([]key{
				key("bar"),
			}))
		})
	})
})
