package tree

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FlamebearerStruct", func() {
	Context("simple case", func() {
		It("deserialize returns the same trie", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))

			f := tree.FlamebearerStruct(1024)
			Expect(f.Names).To(ConsistOf("total", "a", "b", "c"))
			Expect(f.Levels).To(HaveLen(3))
			Expect(f.NumTicks).To(Equal(3))
			Expect(f.MaxSelf).To(Equal(2))
		})
	})
	Context("case with many nodes", func() {
		It("deserialize returns the same trie", func() {
			tree := New()
			r := rand.New(rand.NewSource(123))
			for i := 0; i < 2048; i++ {
				tree.Insert([]byte(fmt.Sprintf("foo;bar%d", i)), uint64(r.Intn(4000)))
			}

			f := tree.FlamebearerStruct(10)
			Expect(f.Names).To(ContainElement("other"))
		})
	})
})
