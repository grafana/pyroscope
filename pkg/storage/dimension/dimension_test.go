package dimension

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/petethepig/pyroscope/pkg/storage/segment"
)

var _ = Describe("dimension", func() {
	Context("Intersection", func() {
		It("works", func() {
			d1 := New()
			d1.Insert(segment.Key("bar"))
			d1.Insert(segment.Key("baz"))
			d1.Insert(segment.Key("foo"))

			d2 := New()
			d2.Insert(segment.Key("foo"))
			d2.Insert(segment.Key("baz"))
			d2.Insert(segment.Key("bar"))

			d3 := New()
			d3.Insert(segment.Key("bar"))

			d4 := New()

			Expect(Intersection(d1, d2, d3, d4)).To(Equal([]segment.Key{}))
			Expect(Intersection(d1, d2, d3)).To(Equal([]segment.Key{segment.Key("bar")}))
			Expect(Intersection(d1, d2)).To(Equal([]segment.Key{
				segment.Key("bar"),
				segment.Key("baz"),
				segment.Key("foo"),
			}))
		})
	})
})
