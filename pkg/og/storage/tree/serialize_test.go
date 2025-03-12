package tree

import (
	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
)

var _ = Describe("tree", func() {
	Describe("Insert", func() {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("correctly sets children nodes", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
		})
	})
})
