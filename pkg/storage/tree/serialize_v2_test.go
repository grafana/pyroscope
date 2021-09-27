package tree

import (
	"bytes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
)

func expectLabel(t *Tree, idx int, label string) {
	Expect(string(t.loadNodeLabel(idx))).To(Equal(label))
}

var _ = Describe("tree", func() {
	Describe("Serialize/Deserialize", func() {
		d := dict.New()
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))
		expected := tree.String()

		var buf bytes.Buffer
		It("reversible", func() {
			Expect(tree.Serialize(d, 1024, &buf)).ToNot(HaveOccurred())
			dt, err := Deserialize(d, bytes.NewReader(buf.Bytes()))
			Expect(err).ToNot(HaveOccurred())
			Expect(dt.String()).To(Equal(expected))
		})
	})
})
