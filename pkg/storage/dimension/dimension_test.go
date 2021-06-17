package dimension

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("dimension", func() {
	Context("Delete", func() {
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

			d1.Delete(Key("baz"))
			d2.Delete(Key("foo"))
			d3.Delete(Key("bar"))

			d1.Delete(Key("x"))
			d2.Delete(Key("x"))
			d3.Delete(Key("x"))

			Expect(Intersection(d1)).To(Equal([]Key{
				Key("bar"),
				Key("foo"),
			}))
			Expect(Intersection(d2)).To(Equal([]Key{
				Key("bar"),
				Key("baz"),
			}))
			Expect(Intersection(d3)).To(Equal([]Key{}))
		})
	})

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

			Expect(Union(d1, d2, d3, d4)).To(ConsistOf([]Key{
				Key("bar"),
				Key("baz"),
				Key("foo"),
			}))
			Expect(Union(d1, d2, d3)).To(ConsistOf([]Key{
				Key("bar"),
				Key("baz"),
				Key("foo"),
			}))
			Expect(Union(d3, d4)).To(ConsistOf([]Key{
				Key("bar"),
			}))
		})
	})
})
