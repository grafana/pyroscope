package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
)

var dictSerializeExample = []byte{1, 0, 0, 1, 2, 0, 1, 0, 2, 2, 1, 1, 1, 0, 2, 2, 1, 2, 0}

var _ = Describe("tree package", func() {
	Describe("Serialize", func() {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("returns correct results", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
		})
	})

	Describe("trie.Serialize(d, )", func() {
		d := dict.New()
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("returns correct results", func() {
			var buf bytes.Buffer
			tree.Serialize(d, 1024, &buf)
			Expect(buf.Bytes()).To(Equal(dictSerializeExample))
		})

		Context("Ran 1000000 times", func() {
			var buf1 bytes.Buffer
			tree.Serialize(d, 1024, &buf1)
			It("returns the same result", func() {
				var buf2 bytes.Buffer
				tree.Serialize(d, 1024, &buf2)
				Expect(buf2).To(Equal(buf1))
			})
		})
	})

	Describe("trie.Deserialize()", func() {
		// TODO: add a case with a real dictionary
		It("returns correct results", func() {
			d := dict.New()
			r := bytes.NewReader(dictSerializeExample)
			t, err := Deserialize(d, r)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(t.root.Name)).To(Equal(""))
			Expect(string(t.root.ChildrenNodes[0].Name)).To(Equal("label not found AAE="))
			Expect(string(t.root.ChildrenNodes[0].ChildrenNodes[0].Name)).To(Equal("label not found AQE="))
			Expect(string(t.root.ChildrenNodes[0].ChildrenNodes[1].Name)).To(Equal("label not found AgE="))
		})
	})
})
