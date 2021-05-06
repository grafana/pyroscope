package transporttrie

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("trie package", func() {
	Context("trie.Diff()", func() {
		It("diffs 2 tries", func() {
			t1 := New()
			t1.Insert([]byte("foo"), uint64(1))
			t1.Insert([]byte("bar"), uint64(2))
			t1.Insert([]byte("baz"), uint64(3))

			t2 := New()
			t2.Insert([]byte("foo"), uint64(3))
			t2.Insert([]byte("bar"), uint64(2))
			t2.Insert([]byte("baz"), uint64(1))

			t4 := New()
			t4.Insert([]byte("foo"), uint64(0))
			t4.Insert([]byte("bar"), uint64(0))
			t4.Insert([]byte("baz"), uint64(2))

			t3 := t1.Diff(t2)

			Expect(t3.String()).To(Equal(t4.String()))
		})
	})
})
