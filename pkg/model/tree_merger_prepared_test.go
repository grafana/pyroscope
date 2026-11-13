package model

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Keep the former implementation as an independent reference, not a second
// production mode. In particular it computes totals through parent pointers.
func mergeTreeBytesReference[N NodeName, I NodeNameI[N]](m *TreeMerger[N, I], b []byte, opts ...TreeMergeOption[N]) error {
	var o treeMergeOption[N]
	for _, f := range opts {
		f(&o)
	}
	t, err := UnmarshalTree[N, I](b)
	if err != nil {
		return err
	}
	if o.formatNodeNames != nil {
		t.FormatNodeNames(o.formatNodeNames)
	}
	m.MergeTree(t)
	return nil
}

func TestTreeMergerPreparedEquivalent(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for _, format := range []bool{false, true} {
		t.Run(fmt.Sprint("format=", format), func(t *testing.T) {
			want, got := NewTreeMerger[FunctionName, FunctionNameI](), NewTreeMerger[FunctionName, FunctionNameI]()
			var opts []TreeMergeOption[FunctionName]
			if format {
				opts = append(opts, WithTreeMergeFormatNodeNames(func(n FunctionName) FunctionName {
					return FunctionName(strings.ReplaceAll(string(n), "leaf-1", "leaf-0"))
				}))
			}
			for iteration := 0; iteration < 40; iteration++ {
				tree := new(FunctionNameTree)
				for i := 0; i < 50; i++ {
					stack := make([]FunctionName, 1+r.Intn(8))
					for j := range stack {
						stack[j] = FunctionName(fmt.Sprintf("leaf-%d", r.Intn(5)))
					}
					tree.InsertStack(int64(r.Intn(13)-4), stack...)
				}
				b := tree.Bytes(-1, nil)
				require.NoError(t, mergeTreeBytesReference(want, b, opts...))
				require.NoError(t, got.MergeTreeBytes(b, opts...))
				require.Equal(t, want.Tree().Bytes(-1, nil), got.Tree().Bytes(-1, nil))
				require.Equal(t, want.IsEmpty(), got.IsEmpty())
			}
		})
	}
	t.Run("empty zero negative and overflow", func(t *testing.T) {
		positive, zero, negative, overflow := new(FunctionNameTree), new(FunctionNameTree), new(FunctionNameTree), new(FunctionNameTree)
		positive.InsertStack(9, "child", "root")
		zero.InsertStack(0, "zero")
		negative.InsertStack(-3, "negative")
		overflow.InsertStack(1<<62, "overflow")
		overflow.InsertStack(1<<62, "overflow")
		inputs := [][]byte{nil, zero.Bytes(-1, nil), positive.Bytes(-1, nil), negative.Bytes(-1, nil), overflow.Bytes(-1, nil)}
		for _, first := range inputs {
			for _, second := range inputs {
				want, got := NewTreeMerger[FunctionName, FunctionNameI](), NewTreeMerger[FunctionName, FunctionNameI]()
				for _, b := range [][]byte{first, second, inputs[2]} {
					require.NoError(t, mergeTreeBytesReference(want, b))
					require.NoError(t, got.MergeTreeBytes(b))
					require.Equal(t, want.Tree().Bytes(-1, nil), got.Tree().Bytes(-1, nil))
				}
			}
		}
	})
	t.Run("location references and collisions", func(t *testing.T) {
		want, got := NewTreeMerger[LocationRefName, LocationRefNameI](), NewTreeMerger[LocationRefName, LocationRefNameI]()
		identity := func(n LocationRefName) LocationRefName { return n }
		opt := WithTreeMergeFormatNodeNames(func(n LocationRefName) LocationRefName {
			if n > 0 {
				return n % 3
			}
			return n
		})
		for i := 0; i < 10; i++ {
			tree := new(LocationRefNameTree)
			tree.InsertStack(7, LocationRefName(i), 3)
			tree.InsertStack(-2, OtherLocationRef, 3)
			b := tree.Bytes(-1, identity)
			require.NoError(t, mergeTreeBytesReference(want, b, opt))
			require.NoError(t, got.MergeTreeBytes(b, opt))
			require.Equal(t, want.Tree().Bytes(-1, identity), got.Tree().Bytes(-1, identity))
		}
	})
}

func TestTreeMergerPreparedMalformedAtomic(t *testing.T) {
	tree := new(FunctionNameTree)
	tree.InsertStack(7, "child", "root")
	valid := tree.Bytes(-1, nil)
	cases := [][]byte{{0}, {255}, append(bytes.Clone(valid), 0), {255, 255, 255, 255, 255, 255, 255, 255, 255, 2}, append([]byte{0, 0}, binary.AppendUvarint(nil, ^uint64(0))...), {10, 'x'}, {0, 0, 1, 0, 0, 0, 0}}
	for i := 1; i < len(valid); i++ {
		cases = append(cases, valid[:i])
	}
	for _, b := range cases {
		for _, seeded := range []bool{false, true} {
			m := NewTreeMerger[FunctionName, FunctionNameI]()
			if seeded {
				require.NoError(t, m.MergeTreeBytes(valid))
			}
			before, empty := m.Tree().Bytes(-1, nil), m.IsEmpty()
			called := false
			require.Error(t, m.MergeTreeBytes(b, WithTreeMergeFormatNodeNames(func(n FunctionName) FunctionName { called = true; return n })), "%x", b)
			require.False(t, called)
			require.Equal(t, empty, m.IsEmpty())
			require.Equal(t, before, m.Tree().Bytes(-1, nil))
		}
	}
}

func TestTreeMergerPreparedOwnsNames(t *testing.T) {
	tree := new(FunctionNameTree)
	tree.InsertStack(7, "child", "root")
	b := tree.Bytes(-1, nil)
	m := NewTreeMerger[FunctionName, FunctionNameI]()
	require.NoError(t, m.MergeTreeBytes(b))
	want := bytes.Clone(m.Tree().Bytes(-1, nil))
	clear(b)
	require.Equal(t, want, m.Tree().Bytes(-1, nil))
}

func TestTreeMergerPreparedNoncanonical(t *testing.T) {
	for _, names := range [][]string{{"a", "z"}, {"z", "a", "z"}, {"z", "z"}} {
		b := []byte{0, 0, byte(len(names))}
		for i, name := range names {
			b = binary.AppendUvarint(b, uint64(len(name)))
			b = append(b, name...)
			b = binary.AppendUvarint(b, uint64(i+1))
			b = append(b, 0)
		}
		want, got := NewTreeMerger[FunctionName, FunctionNameI](), NewTreeMerger[FunctionName, FunctionNameI]()
		require.NoError(t, mergeTreeBytesReference(want, b))
		require.NoError(t, got.MergeTreeBytes(b))
		require.Equal(t, want.Tree().Bytes(-1, nil), got.Tree().Bytes(-1, nil))
		var calls [2][]FunctionName
		for i, merger := range []*TreeMerger[FunctionName, FunctionNameI]{want, got} {
			opt := WithTreeMergeFormatNodeNames(func(n FunctionName) FunctionName {
				calls[i] = append(calls[i], n)
				return FunctionName(fmt.Sprint(len(calls[i])))
			})
			if i == 0 {
				require.NoError(t, mergeTreeBytesReference(merger, b, opt))
			} else {
				require.NoError(t, merger.MergeTreeBytes(b, opt))
			}
		}
		require.Equal(t, calls[0], calls[1])
		require.Equal(t, want.Tree().Bytes(-1, nil), got.Tree().Bytes(-1, nil))
	}
}

func preparedShapedTree(width, depth int) *FunctionNameTree {
	tree := new(FunctionNameTree)
	for i := 0; i < width; i++ {
		var parent *node[FunctionName]
		for d := 0; d < depth; d++ {
			n := &node[FunctionName]{name: FunctionName(fmt.Sprintf("frame-%06d", i)), total: 1, parent: parent}
			if parent == nil {
				tree.root = append(tree.root, n)
			} else {
				parent.children = []*node[FunctionName]{n}
			}
			parent = n
		}
		parent.self = 1
	}
	return tree
}

func TestTreeMergerPreparedShapes(t *testing.T) {
	for _, shape := range []struct{ width, depth int }{{1, 8192}, {4096, 1}, {1024, 128}} {
		t.Run(fmt.Sprint(shape), func(t *testing.T) {
			b := preparedShapedTree(shape.width, shape.depth).Bytes(-1, nil)
			_, canonical, err := prepareTreeBytes[FunctionName, FunctionNameI](b, true)
			require.NoError(t, err)
			require.True(t, canonical)
			want, got := NewTreeMerger[FunctionName, FunctionNameI](), NewTreeMerger[FunctionName, FunctionNameI]()
			var calls [2][]FunctionName
			for i := 0; i < 2; i++ {
				opt := WithTreeMergeFormatNodeNames(func(n FunctionName) FunctionName {
					calls[i] = append(calls[i], n)
					return n
				})
				require.NoError(t, mergeTreeBytesReference(want, b, opt))
			}
			var gotCalls []FunctionName
			for i := 0; i < 2; i++ {
				require.NoError(t, got.MergeTreeBytes(b, WithTreeMergeFormatNodeNames(func(n FunctionName) FunctionName {
					gotCalls = append(gotCalls, n)
					return n
				})))
			}
			require.Equal(t, append(calls[0], calls[1]...), gotCalls)
			require.Equal(t, want.Tree().Bytes(-1, nil), got.Tree().Bytes(-1, nil))
		})
	}
}

func TestTreeMergerPreparedFixturesAndConcurrent(t *testing.T) {
	inputs := preparedFixtureInputs(t)
	want := NewTreeMerger[FunctionName, FunctionNameI]()
	for i := 0; i < 8; i++ {
		require.NoError(t, mergeTreeBytesReference(want, inputs[i%len(inputs)]))
	}
	got := NewTreeMerger[FunctionName, FunctionNameI]()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				tree, err := UnmarshalTree[FunctionName, FunctionNameI](inputs[i%len(inputs)])
				if err != nil {
					errs <- err
					return
				}
				got.MergeTree(tree)
			} else {
				errs <- got.MergeTreeBytes(inputs[i%len(inputs)])
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, want.Tree().Bytes(-1, nil), got.Tree().Bytes(-1, nil))
}

func preparedFixtureInputs(tb testing.TB) [][]byte {
	tb.Helper()
	var inputs [][]byte
	for _, file := range []string{"diff_left_tree.bin", "diff_right_tree.bin"} {
		b, err := os.ReadFile("testdata/" + file)
		require.NoError(tb, err)
		inputs = append(inputs, b)
	}
	return inputs
}

func BenchmarkTreeMergerPrepared(b *testing.B) {
	inputs := preparedFixtureInputs(b)
	for _, workers := range []int{1, 4, 8} {
		for _, candidate := range []bool{false, true} {
			b.Run(fmt.Sprintf("workers=%d/prepared=%t", workers, candidate), func(b *testing.B) {
				b.ReportAllocs()
				for op := 0; op < b.N; op++ {
					m := NewTreeMerger[FunctionName, FunctionNameI]()
					var wg sync.WaitGroup
					for w := 0; w < workers; w++ {
						wg.Add(1)
						go func(w int) {
							defer wg.Done()
							for i := w; i < 16; i += workers {
								var err error
								if candidate {
									err = m.MergeTreeBytes(inputs[i%2])
								} else {
									err = mergeTreeBytesReference(m, inputs[i%2])
								}
								if err != nil {
									b.Error(err)
								}
							}
						}(w)
					}
					wg.Wait()
				}
			})
		}
	}
}

func BenchmarkTreeMergerPrepared131k(b *testing.B) {
	input := preparedShapedTree(1024, 128).Bytes(-1, nil)
	for _, candidate := range []bool{false, true} {
		b.Run(fmt.Sprint("prepared=", candidate), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				m := NewTreeMerger[FunctionName, FunctionNameI]()
				for j := 0; j < 4; j++ {
					var err error
					if candidate {
						err = m.MergeTreeBytes(input)
					} else {
						err = mergeTreeBytesReference(m, input)
					}
					require.NoError(b, err)
				}
			}
		})
	}
}

func FuzzTreeMergerPreparedMalformed(f *testing.F) {
	for _, b := range [][]byte{nil, {0, 0, 0}, {0, 0, 1, 0, 1, 0}, {255}, {0, 0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 1}} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 32768 {
			t.Skip()
		}
		checkPreparedEncoding[FunctionName, FunctionNameI](t, b)
		checkPreparedEncoding[LocationRefName, LocationRefNameI](t, b)
	})
}

func checkPreparedEncoding[N NodeName, I NodeNameI[N]](t *testing.T, b []byte) {
	t.Helper()
	identity := func(n N) N { return n }
	want, got := NewTreeMerger[N, I](), NewTreeMerger[N, I]()
	seed := []byte{0, 0, 1, 0, 1, 0}
	require.NoError(t, got.MergeTreeBytes(seed))
	before := got.Tree().Bytes(-1, identity)
	if err := got.MergeTreeBytes(b); err != nil {
		require.Equal(t, before, got.Tree().Bytes(-1, identity))
		return
	}
	require.NoError(t, mergeTreeBytesReference(want, seed))
	require.NoError(t, mergeTreeBytesReference(want, b))
	require.Equal(t, want.Tree().Bytes(-1, identity), got.Tree().Bytes(-1, identity))
}

func FuzzTreeMergerPreparedEquivalent(f *testing.F) {
	f.Add([]byte{1, 3, 7, 0, 4, 9})
	f.Add([]byte{255, 0, 128, 17})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			t.Skip()
		}
		want, got := NewTreeMerger[FunctionName, FunctionNameI](), NewTreeMerger[FunctionName, FunctionNameI]()
		for k := 0; k < 2; k++ {
			tree := new(FunctionNameTree)
			for i, v := range data {
				tree.InsertStack(int64(int8(v)), FunctionName(fmt.Sprint(v%5)), FunctionName(fmt.Sprint((i+k)%7)), "root")
			}
			b := tree.Bytes(-1, nil)
			require.NoError(t, mergeTreeBytesReference(want, b))
			require.NoError(t, got.MergeTreeBytes(b))
			require.Equal(t, want.Tree().Bytes(-1, nil), got.Tree().Bytes(-1, nil))
		}
	})
}
