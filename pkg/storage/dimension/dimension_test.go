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

			Expect(Intersection(d1)).To(ConsistOf([]Key{
				Key("bar"),
				Key("foo"),
			}))
			Expect(Intersection(d2)).To(ConsistOf([]Key{
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

			Expect(Intersection(d1, d2, d3, d4)).To(BeNil())
			Expect(Intersection(d1, d2, d3)).To(Equal([]Key{Key("bar")}))
			Expect(Intersection(d1, d2)).To(ConsistOf([]Key{
				Key("bar"),
				Key("baz"),
				Key("foo"),
			}))
		})

		It("works correctly", func() {
			d1 := New()
			v := []string{
				"ride-sharing-app.cpu{hostname=40dfbd6616c3,region=us-west-1,vehicle=scooter}",
				"ride-sharing-app.cpu{hostname=4ef76b35f112,region=us-east-1,vehicle=scooter}",
				"ride-sharing-app.cpu{hostname=5fec370dfb99,region=eu-west-1,vehicle=scooter}",
				"ride-sharing-app.cpu{hostname=680222ce937b,region=eu-west-1,vehicle=scooter}",
				"ride-sharing-app.cpu{hostname=904af763ff84,region=us-east-1,vehicle=scooter}",
				"ride-sharing-app.cpu{hostname=a985ede2759f,region=us-west-1,vehicle=scooter}",
			}
			for _, k := range v {
				d1.Insert(Key(k))
			}

			d2 := New()
			v = []string{
				"ride-sharing-app.cpu{hostname=5fec370dfb99,region=eu-west-1,vehicle=bike}",
				"ride-sharing-app.cpu{hostname=5fec370dfb99,region=eu-west-1,vehicle=car}",
				"ride-sharing-app.cpu{hostname=5fec370dfb99,region=eu-west-1,vehicle=scooter}",
				"ride-sharing-app.cpu{hostname=5fec370dfb99,region=eu-west-1}",
				"ride-sharing-app.cpu{hostname=680222ce937b,region=eu-west-1,vehicle=bike}",
				"ride-sharing-app.cpu{hostname=680222ce937b,region=eu-west-1,vehicle=car}",
				"ride-sharing-app.cpu{hostname=680222ce937b,region=eu-west-1,vehicle=scooter}",
				"ride-sharing-app.cpu{hostname=680222ce937b,region=eu-west-1}",
			}
			for _, k := range v {
				d2.Insert(Key(k))
			}

			Expect(Intersection(d1, d2)).To(ConsistOf([]Key{
				Key("ride-sharing-app.cpu{hostname=5fec370dfb99,region=eu-west-1,vehicle=scooter}"),
				Key("ride-sharing-app.cpu{hostname=680222ce937b,region=eu-west-1,vehicle=scooter}"),
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
