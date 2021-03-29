package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var serializationExample = []byte{0, 0, 1, 1, 97, 0, 2, 1, 98, 1, 0, 1, 99, 2, 0}

var _ = Describe("tree package", func() {
	Describe("SerializeNoDict", func() {
		It("returns correct results", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))

			var buf bytes.Buffer
			tree.SerializeNoDict(1024, &buf)
			Expect(buf.Bytes()).To(Equal(serializationExample))
		})
	})

	Describe("DeserializeNoDict", func() {
		It("returns correct results", func() {
			r := bytes.NewReader(serializationExample)
			t, err := DeserializeNoDict(r)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(t.root.Name)).To(Equal(""))
			Expect(string(t.root.ChildrenNodes[0].Name)).To(Equal("a"))
			Expect(string(t.root.ChildrenNodes[0].ChildrenNodes[0].Name)).To(Equal("b"))
			Expect(string(t.root.ChildrenNodes[0].ChildrenNodes[1].Name)).To(Equal("c"))
		})
	})
})
