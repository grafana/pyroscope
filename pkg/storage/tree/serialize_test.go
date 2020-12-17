package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	log "github.com/sirupsen/logrus"
)

var serializationExample = []byte{4, 0, 0, 0, 0, 0, 1, 5, 97, 60, 37, 105, 178, 0, 2, 5, 98, 149, 222, 126, 3, 1, 0, 5, 99, 225, 50, 214, 95, 2, 0}

// var serializationExample = []byte{4, 0, 0, 0, 0, 0, 1, 5, 97, 60, 37, 105, 178, 0, 2, 5, 98, 149, 222, 126, 3, 1, 0, 5, 99, 225, 50, 214, 95, 2, 0}

var _ = Describe("tree package", func() {
	Context("Serialize", func() {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("returns correct results", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
		})

	})

	Context("trie.Serialize(d, )", func() {
		d := dict.New()
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		XIt("returns correct results", func() {
			var buf bytes.Buffer
			tree.Serialize(d, 1024, &buf)
			Expect(buf.Bytes()).To(Equal(serializationExample))
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

	Context("trie.Deserialize()", func() {
		It("returns correct results", func() {
			d := dict.New()
			r := bytes.NewReader(serializationExample)
			t, err := Deserialize(d, r)
			Expect(err).ToNot(HaveOccurred())
			Expect(t.String()).ToNot(Equal("test"))
		})
	})

	Context("trie.Ser/Deserialize()", func() {
		d := dict.New()
		d2 := dict.New()
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		XIt("deserialize returns the same trie", func() {
			var buf bytes.Buffer
			tree.Serialize(d, 1024, &buf)
			b := buf.Bytes()
			Expect(b).To(Equal(serializationExample))
			t2 := FromBytes(d2, b)

			log.Debug("tree", tree.String())
			log.Debug("t2", t2.String())
			Expect(tree.String()).To(Equal(t2.String()))
			Expect(nil).ToNot(BeNil())
		})
	})
})
