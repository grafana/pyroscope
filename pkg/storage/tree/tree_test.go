package tree

import (
	"bytes"
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
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
			Expect(tree.String()).To(Equal("\"a;b\" 1\n\"a;c\" 2\n"))
		})
	})

	Context("Merge", func() {
		Context("similar trees", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;c"), uint64(8))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;c" 8|`)))
			})

			It("properly merges", func() {
				treeA.Merge(treeB)

				Expect(treeA.root.ChildrenNodes).To(HaveLen(1))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
				Expect(treeA.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(treeA.root.ChildrenNodes[0].Total).To(Equal(uint64(15)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(10)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(10)))
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

				Expect(treeA.root.ChildrenNodes).To(HaveLen(1))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(4))
				Expect(treeA.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(treeA.root.ChildrenNodes[0].Total).To(Equal(uint64(30)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[2].Self).To(Equal(uint64(8)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[3].Self).To(Equal(uint64(15)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[2].Total).To(Equal(uint64(8)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[3].Total).To(Equal(uint64(15)))
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 5|"a;c" 2|"a;d" 8|"a;e" 15|`)))
			})
		})
	})
})

func treeStr(s string) string {
	return strings.ReplaceAll(s, "|", "\n")
}

var _ = Describe("prepend", func() {
	Context("prependTreeNode)", func() {
		It("prepend elem", func() {
			A, B, C, X := &treeNode{}, &treeNode{}, &treeNode{}, &treeNode{}
			s := []*treeNode{A, B, C}
			s = prependTreeNode(s, X)
			Expect(s).To(HaveLen(4))
			Expect(s[0]).To(Equal(X))
			Expect(s[1]).To(Equal(A))
			Expect(s[2]).To(Equal(B))
			Expect(s[3]).To(Equal(C))
		})
	})
	Context("prependBytes", func() {
		It("prepend elem", func() {
			A, B, C, X := []byte("A"), []byte("B"), []byte("C"), []byte("X")
			s := [][]byte{A, B, C}
			s = prependBytes(s, X)

			out := bytes.Join(s, []byte(","))
			Expect(string(out)).To(Equal("X,A,B,C"))
		})
	})
})
