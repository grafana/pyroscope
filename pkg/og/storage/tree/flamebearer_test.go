package tree

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
)

var _ = Describe("FlamebearerStruct", func() {
	Context("simple case", func() {
		It("sets all attributes correctly", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))

			f := tree.FlamebearerStruct(1024)
			Expect(f.Names).To(Equal([]string{"total", "a", "c", "b"}))
			Expect(f.Levels).To(Equal([][]int{
				// i+0 = x offset (delta encoded)
				// i+1 = total
				// i+2 = self
				// i+3 = index in names array
				{0, 3, 0, 0},
				{0, 3, 0, 1},
				{0, 1, 1, 3, 0, 2, 2, 2},
			}))
			Expect(f.NumTicks).To(Equal(3))
			Expect(f.MaxSelf).To(Equal(2))
		})
	})
	Context("case with many nodes", func() {
		It("groups nodes into a new \"other\" node", func() {
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
