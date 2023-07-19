package tree

import (
	"github.com/grafana/pyroscope/pkg/og/structs/cappedarr"
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
