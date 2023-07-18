package tree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
)

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
			Expect(tree.String()).To(Equal("a;b 1\na;c 2\n"))
		})
	})

	Context("Diff", func() {
		a := New()
		a.Insert([]byte("a;b;c"), uint64(100))
		a.Insert([]byte("a;b;c;d"), uint64(100))
		a.Insert([]byte("a;b;d"), uint64(100))
		a.Insert([]byte("a;e"), uint64(100))
		a.Insert([]byte("a;f"), uint64(150))
		a.Insert([]byte("a;h"), uint64(150))

		b := New()
		b.Insert([]byte("a;b;c"), uint64(120))
		b.Insert([]byte("a;b;c;d"), uint64(120))
		b.Insert([]byte("a;b;d"), uint64(120))
		b.Insert([]byte("a;e"), uint64(100))
		b.Insert([]byte("a;f"), uint64(150))
		b.Insert([]byte("a;g"), uint64(20))
		b.Insert([]byte("a;h"), uint64(170))

		diff := a.Diff(b)
		z, _ := json.MarshalIndent(diff, "", "\t")
		fmt.Println(string(z))
		It("properly sets up a tree", func() {
			Expect(diff).To(beTree([]stack{
				{"a;g", 20},
				{"a;h", 20},
				{"a;b;c", 20},
				{"a;b;d", 20},
				{"a;b;c;d", 20},
			}))
		})
	})

	Context("InsertStackString unsorted of length 1", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "a"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(1)))
			Expect(tree.String()).To(Equal("a;a 2\na;b 1\n"))
		})
	})

	Context("InsertStackString equal of length 1", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "b"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.String()).To(Equal("a;b 3\n"))
		})
	})

	Context("InsertStackString sorted of length 1", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "c"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
			Expect(tree.String()).To(Equal("a;b 1\na;c 2\n"))
		})
	})

	Context("InsertStackString sorted of different lengths", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "b"}, uint64(1))
		tree.InsertStackString([]string{"a", "ba"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
			Expect(tree.String()).To(Equal("a;b 1\na;ba 2\n"))
		})
	})

	Context("InsertStackString unsorted of different lengths", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "ba"}, uint64(1))
		tree.InsertStackString([]string{"a", "b"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(1)))
			Expect(tree.String()).To(Equal("a;b 2\na;ba 1\n"))
		})
	})

	Context("InsertStackString unsorted of length 2", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "bb"}, uint64(1))
		tree.InsertStackString([]string{"a", "ba"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(1)))
			Expect(tree.String()).To(Equal("a;ba 2\na;bb 1\n"))
		})
	})

	Context("InsertStackString equal of length 2", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "bb"}, uint64(1))
		tree.InsertStackString([]string{"a", "bb"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.String()).To(Equal("a;bb 3\n"))
		})
	})

	Context("InsertStackString sorted of length 2", func() {
		tree := New()
		tree.InsertStackString([]string{"a", "bb"}, uint64(1))
		tree.InsertStackString([]string{"a", "bc"}, uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
			Expect(tree.String()).To(Equal("a;bb 1\na;bc 2\n"))
		})
	})

	Context("Merge", func() {
		Context("similar trees", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`a;b 1|a;c 2|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;c"), uint64(8))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`a;b 4|a;c 8|`)))
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
				Expect(treeA.String()).To(Equal(treeStr(`a;b 5|a;c 10|`)))
			})
		})

		Context("tree with an extra node", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			treeA.Insert([]byte("a;e"), uint64(3))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`a;b 1|a;c 2|a;e 3|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;d"), uint64(8))
			treeB.Insert([]byte("a;e"), uint64(12))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`a;b 4|a;d 8|a;e 12|`)))
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
				Expect(treeA.String()).To(Equal(treeStr(`a;b 5|a;c 2|a;d 8|a;e 15|`)))
			})
		})

		Context("tree.scale", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			treeA.Insert([]byte("a;e"), uint64(3))
			treeA.Insert([]byte("a"), uint64(4))
			treeA.Scale(3)
			It("", func() {
				Expect(treeA.String()).To(Equal(treeStr(`a 12|a;b 3|a;c 6|a;e 9|`)))
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

type BeTreeMatcher struct {
	Expected string
}

type stack struct {
	Name  string
	Value int
}

func beTree(stacks []stack) *BeTreeMatcher {
	var b strings.Builder
	for _, s := range stacks {
		_, _ = fmt.Fprintf(&b, "%s %d\n", s.Name, s.Value)
	}
	return &BeTreeMatcher{Expected: b.String()}
}

func (m *BeTreeMatcher) Match(actual interface{}) (success bool, err error) {
	t, ok := actual.(*Tree)
	if !ok {
		return false, nil
	}
	return t.String() == m.Expected, nil
}

func (m *BeTreeMatcher) FailureMessage(actual interface{}) string {
	return format.Message(actual.(*Tree).String(), "to be", m.Expected)
}

func (m *BeTreeMatcher) NegatedFailureMessage(actual interface{}) string {
	return format.Message(actual.(*Tree).String(), "not to be", m.Expected)
}
