package tree

import (
	"github.com/pyroscope-io/pyroscope/pkg/structs/cappedarr"
)

func (t *Tree) minValue(maxNodes int) uint64 {
	if maxNodes == -1 {
		return 0
	}
	c := cappedarr.New(maxNodes)
	t.iterateWithTotal(func(total uint64) bool {
		return c.Push(total)
	})
	return c.MinValue()
}
