//go:build !goexperiment.arenas

package tree

func (t *Tree) InsertStackA(stack [][]byte, v uint64) {
	t.InsertStack(stack, v)
}
