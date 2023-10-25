package iter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_AsyncBatchIter(t *testing.T) {
	type testCase struct {
		description string
		seqSize     int
		bufSize     int
	}
	testCases := []testCase{
		{
			description: "empty iterator",
			seqSize:     0,
			bufSize:     1,
		},
		{
			description: "empty iterator, zero buffer",
			seqSize:     0,
			bufSize:     0,
		},
		{
			description: "zero buffer",
			seqSize:     10,
			bufSize:     0,
		},
		{
			description: "iterator < buffer",
			seqSize:     5,
			bufSize:     10,
		},
		{
			description: "iterator == buffer",
			seqSize:     10,
			bufSize:     10,
		},
		{
			description: "iterator > buffer",
			seqSize:     25,
			bufSize:     10,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			x := NewAsyncBatchIterator[int, int](
				newSeqIterator(tc.seqSize),
				tc.bufSize,
				func(i int) int { return i },
				func([]int) {},
			)
			var p, c int
			for x.Next() {
				i := x.At()
				require.Equal(t, 1, i-p)
				p = i
				c++
			}
			require.Equal(t, tc.seqSize, c)
			require.NoError(t, x.Err())
			require.NoError(t, x.Close())
		})
	}
}

type seqIterator struct{ n, c int }

func newSeqIterator(n int) *seqIterator {
	return &seqIterator{n: n}
}

func (x *seqIterator) Next() bool {
	if x.c < x.n {
		x.c++
		return true
	}
	return false
}

func (x *seqIterator) At() int { return x.c }

func (x *seqIterator) Close() error { return nil }
func (x *seqIterator) Err() error   { return nil }

// Benchmark_AsyncBatchIterator-10    	   91417	     13353 ns/op	   17017 B/op	      10 allocs/op
func Benchmark_AsyncBatchIterator(b *testing.B) {
	b.ReportAllocs()
	var n int
	for i := 0; i < b.N; i++ {
		x := NewAsyncBatchIterator[int, int](
			newSeqIterator(1<<20),
			1<<10,
			func(i int) int { return i },
			func([]int) {},
		)
		for x.Next() {
			n += x.At()
		}
	}
}

// Benchmark_BufferedIterator-10    	      12	  99730976 ns/op	   10047 B/op	       8 allocs/op
func Benchmark_BufferedIterator(b *testing.B) {
	b.ReportAllocs()
	var n int
	for i := 0; i < b.N; i++ {
		x := NewBufferedIterator[int](newSeqIterator(1<<20), 1<<10)
		for x.Next() {
			n += x.At()
		}
	}
}
