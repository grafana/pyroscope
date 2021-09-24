package tree

import (
	"bufio"
	"bytes"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

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

var (
	rawTreeA = mustParse("testdata/tree_1.txt")
	rawTreeB = mustParse("testdata/tree_2.txt")
)

type line struct {
	key   []byte
	value uint64
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
		Context("creates a tree copy", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))
			Expect(tree.Clone(big.NewRat(2, 1)).String()).
				To(Equal("\"a;b\" 2\n\"a;c\" 4\n"))
		})
	})

	Context("MarshalJSON", func() {
		Context("creates a tree copy", func() {
			tree := New()
			tree.Insert([]byte("a;b"), uint64(1))
			tree.Insert([]byte("a;c"), uint64(2))

			s, err := tree.MarshalJSON()
			Expect(err).ToNot(HaveOccurred())
			Expect(string(s)).To(Equal(`{"name":"","total":3,"self":0,"children":[{"name":"a","total":3,"self":0,"children":[{"name":"b","total":1,"self":1,"children":[]},{"name":"c","total":2,"self":2,"children":[]}]}]}`))
		})
	})
})

func BenchmarkClone(b *testing.B) {
	r := big.NewRat(1, 1)
	for i := 0; i < b.N; i++ {
		tree := New()
		for _, l := range rawTreeB {
			tree.Insert(l.key, l.value)
		}
		c := tree.Clone(r)
		tree.Reset()
		c.Reset()
	}
}

func BenchmarkMerge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		treeA := New()
		for _, l := range rawTreeA {
			treeA.Insert(l.key, l.value)
		}
		treeB := New()
		for _, l := range rawTreeB {
			treeB.Insert(l.key, l.value)
		}
		treeA.Merge(treeB)
		treeA.Reset()
		treeB.Reset()
	}
}

func treeStr(s string) string {
	return strings.ReplaceAll(s, "|", "\n")
}

func mustParse(path string) (lines []line) {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = f.Close()
	}()
	w := []byte{' '}
	s := bufio.NewScanner(bufio.NewReader(f))
	for s.Scan() {
		i := bytes.LastIndex(s.Bytes(), w)
		n, err := strconv.Atoi(s.Text()[i+1:])
		if err != nil {
			panic(err)
		}
		lines = append(lines, line{
			key:   []byte(s.Text())[:i],
			value: uint64(n),
		})
	}
	return lines
}
