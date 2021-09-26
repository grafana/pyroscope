package tree

import (
	"math/big"
	"math/rand"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func randStr() []byte {
	buf := make([]byte, 10)
	for i := 0; i < 10; i++ {
		buf[i] = byte(97) + byte(rand.Uint32()%10)
	}
	return buf
}

var _ = Describe("tree package", func() {
	Context("Insert", func() {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.String()).To(Equal("\"a;b\" 1\n\"a;c\" 2\n"))
		})
	})

	Context("Merge", func() {
		Context("similar trees", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;c"), uint64(8))

			It("properly merges", func() {
				treeA.Merge(treeB)
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 5|"a;c" 10|`)))
			})
		})

		Context("tree with an extra node", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			treeA.Insert([]byte("a;e"), uint64(3))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|"a;e" 3|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;d"), uint64(8))
			treeB.Insert([]byte("a;e"), uint64(12))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;d" 8|"a;e" 12|`)))
			})

			It("properly merges", func() {
				treeA.Merge(treeB)

				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 5|"a;c" 2|"a;d" 8|"a;e" 15|`)))
			})
		})
	})

	Context("Clone", func() {
		It("creates a tree copy", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))
			Expect(tree.Clone(big.NewRat(2, 1)).String()).
				To(Equal(treeStr(`"a;b" 2|"a;c" 4|`)))
		})
	})

	Context("MarshalJSON", func() {
		It("creates an expected JSON output", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))

			s, err := tree.MarshalJSON()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(s)).To(Equal(`{"name":"","total":3,"self":0,"children":[{"name":"a","total":3,"self":0,"children":[{"name":"b","total":1,"self":1,"children":[]},{"name":"c","total":2,"self":2,"children":[]}]}]}`))
		})
	})

	Context("MinValue", func() {
		It("returns expected value", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))

			Expect(tree.minValue(0)).To(Equal(uint64(3)))
			Expect(tree.minValue(1)).To(Equal(uint64(3)))
			Expect(tree.minValue(2)).To(Equal(uint64(2)))
			Expect(tree.minValue(3)).To(Equal(uint64(1)))
			Expect(tree.minValue(4)).To(Equal(uint64(0)))
		})
	})

	Context("Truncate", func() {
		It("returns expected value", func() {
			tree := New()
			tree.Insert([]byte("foo;baz"), uint64(3))
			tree.Insert([]byte("foo;bar;a"), uint64(1))
			tree.Insert([]byte("foo;bar;b"), uint64(1))
			tree.Insert([]byte("foo;bar;c"), uint64(1))
			Expect(tree.Len()).To(Equal(7))
			tree.Truncate(3)
			Expect(tree.Len()).To(Equal(4))
			Expect(tree.String()).To(Equal(treeStr(`"foo;bar" 3|"foo;baz" 3|`)))
		})
	})
})

func treeStr(s string) string {
	return strings.ReplaceAll(s, "|", "\n")
}
