package dict

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var serialized = []byte{1, 0, 2, 3, 102, 111, 111, 1, 3, 98, 97, 114, 0, 3, 98, 97, 114, 0}

var _ = Describe("dict", func() {
	Describe("Serialize", func() {
		It("returns correct results", func() {
			dict := New()
			Expect(dict.Put(Value("foo"))).To(Equal(Key{0, 3}))
			Expect(dict.Put(Value("bar"))).To(Equal(Key{1, 3}))
			Expect(dict.Put(Value("foobar"))).To(Equal(Key{0, 3, 0, 3}))

			var b bytes.Buffer
			dict.Serialize(&b)
			Expect(b.Bytes()).To(Equal(serialized))
		})
	})

	Describe("Deserialize", func() {
		// TODO: add a case with a real dictionary
		It("returns correct results", func() {
			r := bytes.NewReader(serialized)
			d, err := Deserialize(r)
			Expect(err).ToNot(HaveOccurred())
			v1, _ := d.Get(Key{0, 3})
			Expect(v1).To(Equal(Value("foo")))
			v2, _ := d.Get(Key{1, 3})
			Expect(v2).To(Equal(Value("bar")))
			v3, _ := d.Get(Key{0, 3, 0, 3})
			Expect(v3).To(Equal(Value("foobar")))
		})
	})
})
