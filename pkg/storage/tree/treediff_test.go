package tree

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// StringWithEmpty is used for testing only, hence declared in this file. It's
// different to String() as it includes nodes with zero value.
func (t *Tree) StringWithEmpty() string {
	t.RLock()
	defer t.RUnlock()

	res := ""
	t.Iterate(func(k []byte, v uint64) {
		if len(k) >= 2 {
			res += fmt.Sprintf("%q %d\n", k[2:], v)
		}
	})
	return res
}

var _ = Describe("tree package", func() {
	Context("diff", func() {
		Context("similar trees", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			It("properly sets up tree A", func() {
				Expect(treeA.StringWithEmpty()).To(Equal(treeStr(`"a" 0|"a;b" 1|"a;c" 2|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;c"), uint64(8))
			It("properly sets up tree B", func() {
				Expect(treeB.StringWithEmpty()).To(Equal(treeStr(`"a" 0|"a;b" 4|"a;c" 8|`)))
			})

			It("properly combine trees", func() {
				CombineTree(treeA, treeB)

				Expect(treeA.StringWithEmpty()).To(Equal(treeStr(`"a" 0|"a;b" 1|"a;c" 2|`)))
				Expect(treeB.StringWithEmpty()).To(Equal(treeStr(`"a" 0|"a;b" 4|"a;c" 8|`)))
			})

			It("properly combine trees to flamebearer", func() {
				f := CombineToFlamebearerStruct(treeA, treeB, 1024)

				Expect(f.Names).To(ConsistOf("total", "a", "b", "c"))
				Expect(f.Levels).To(Equal([][]int{
					// i+0 = x offset, left  tree
					// i+1 = total   , left  tree
					// i+2 = self    , left  tree
					// i+3 = x offset, right tree
					// i+4 = total   , right tree
					// i+5 = self    , right tree
					// i+6 = index in the names array
					{0, 3, 0, 0, 12, 0, 0},
					{0, 3, 0, 0, 12, 0, 1},
					{0, 1, 1, 0, 4, 4, 3, 0, 2, 2, 0, 8, 8, 2},
				}))
				Expect(f.NumTicks).To(Equal(15))
				Expect(f.MaxSelf).To(Equal(8))
			})
		})

		Context("tree with an extra node", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			treeA.Insert([]byte("a;e"), uint64(3))
			It("properly sets up tree A", func() {
				Expect(treeA.StringWithEmpty()).To(Equal(treeStr(`"a" 0|"a;b" 1|"a;c" 2|"a;e" 3|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;d"), uint64(8))
			treeB.Insert([]byte("a;e"), uint64(12))
			It("properly sets up tree B", func() {
				Expect(treeB.StringWithEmpty()).To(Equal(treeStr(`"a" 0|"a;b" 4|"a;d" 8|"a;e" 12|`)))
			})

			It("properly combine trees", func() {
				CombineTree(treeA, treeB)

				expectedA := `"a" 0|"a;b" 1|"a;c" 2|"a;d" 0|"a;e" 3|`
				expectedB := `"a" 0|"a;b" 4|"a;c" 0|"a;d" 8|"a;e" 12|`
				Expect(treeA.StringWithEmpty()).To(Equal(treeStr(expectedA)))
				Expect(treeB.StringWithEmpty()).To(Equal(treeStr(expectedB)))
			})

			It("properly combine trees to flamebearer", func() {
				f := CombineToFlamebearerStruct(treeA, treeB, 1024)

				Expect(f.Names).To(ConsistOf("total", "a", "b", "c", "d", "e"))
				Expect(f.Levels).To(Equal([][]int{
					// i+0 = x offset, left  tree
					// i+1 = total   , left  tree
					// i+2 = self    , left  tree
					// i+3 = x offset, right tree
					// i+4 = total   , right tree
					// i+5 = self    , right tree
					// i+6 = index in the names array
					{0, 6, 0, 0, 24, 0, 0}, // total
					{0, 6, 0, 0, 24, 0, 1}, //    a
					{
						0, 1, 1, 0, 4, 4, 5, //   e
						0, 2, 2, 0, 0, 0, 4, //   d
						0, 0, 0, 0, 8, 8, 3, //   c
						0, 3, 3, 0, 12, 12, 2, // b
					},
				}))
				Expect(f.NumTicks).To(Equal(30))
				Expect(f.MaxSelf).To(Equal(12))
			})
		})

		Context("tree with many nodes", func() {
			It("groups nodes into a new \"other\" node", func() {
				treeA, treeB := New(), New()
				r := rand.New(rand.NewSource(123))
				for i := 0; i < 2048; i++ {
					treeA.Insert([]byte(fmt.Sprintf("foo;bar%d", i)), uint64(r.Intn(4000)))
					treeB.Insert([]byte(fmt.Sprintf("foo;bar%d", i)), uint64(r.Intn(4000)))
				}

				f := CombineToFlamebearerStruct(treeA, treeB, 10)
				Expect(f.Names).To(ContainElement("other"))
			})
		})
	})
})
