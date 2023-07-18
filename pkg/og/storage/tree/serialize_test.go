package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"github.com/grafana/pyroscope/pkg/og/storage/dict"
)

var dictSerializeExample = []byte("\x01\x00\x00\x01\x02\x00\x01\x00\x02\x02\x01\x01\x01\x00\x02\x02\x01\x02\x00")

var _ = Describe("tree", func() {
	Describe("Insert", func() {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("correctly sets children nodes", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
		})
	})

	Describe("SerializeTruncate", func() {
		d := dict.New()
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("serializes tree", func() {
			var buf bytes.Buffer
			tree.SerializeTruncate(d, 1024, &buf)
			Expect(buf.Bytes()).To(Equal(dictSerializeExample))
		})

		Context("Ran 1000000 times", func() {
			var buf1 bytes.Buffer
			tree.SerializeTruncate(d, 1024, &buf1)
			It("returns the same result", func() {
				var buf2 bytes.Buffer
				tree.SerializeTruncate(d, 1024, &buf2)
				Expect(buf2).To(Equal(buf1))
			})
		})
	})

	Describe("Deserialize", func() {
		// TODO: add a case with a real dictionary
		It("returns a properly deserialized tree", func() {
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
