package tree

import (
	"bufio"
	"bytes"
	"math/big"
	"os"
	"strconv"
	"testing"
)

var (
	rawTreeA = mustParse("testdata/tree_1.txt")
	rawTreeB = mustParse("testdata/tree_2.txt")
)

type line struct {
	key   []byte
	value uint64
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

func BenchmarkInsert(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tree := New()
		for _, l := range rawTreeB {
			tree.Insert(l.key, l.value)
		}
		tree.Reset()
	}
}

func BenchmarkClone(b *testing.B) {
	r := big.NewRat(1, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree := New()
		for _, l := range rawTreeB {
			tree.Insert(l.key, l.value)
		}
		tree.Clone(r).Reset()
		tree.Reset()
	}
}

func BenchmarkMerge(b *testing.B) {
	treeA := New()
	for _, l := range rawTreeA {
		treeA.Insert(l.key, l.value)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		treeB := New()
		for _, l := range rawTreeB {
			treeA.Insert(l.key, l.value)
		}
		treeB.Merge(treeA)
		treeB.Reset()
	}
	treeA.Reset()
}

func BenchmarkTruncate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tree := New()
		for _, l := range rawTreeB {
			tree.Insert(l.key, l.value)
		}
		// tree.Len() == 2083
		tree.Truncate(2048)
		tree.Reset()
	}
}
