//go:build !goexperiment.arenas

package tree

import "github.com/pyroscope-io/pyroscope/pkg/util"

func (t *Tree) InsertStackA(a *util.ArenaWrapper, stack [][]byte, v uint64) {
	t.InsertStack(stack, v)
}
