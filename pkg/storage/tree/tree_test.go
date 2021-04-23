package tree

import (
	"math/rand"

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
})
