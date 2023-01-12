//go:build goexperiment.arenas

package streaming

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/arenahelper"
)

func (c *LabelsCache) newCacheEntryA(l Labels) (int, *tree.Tree) {
	a := c.arena
	if a == nil {
		return c.newCacheEntry(l)
	}
	from := len(c.labels)
	for _, u := range l {
		c.labels = arenahelper.AppendA(c.labels, u, a)
	}
	to := len(c.labels)
	res := len(c.labelRefs)
	c.labelRefs = arenahelper.AppendA(c.labelRefs, uint64(from<<32|to), a)
	t := tree.NewA(a)
	c.trees = arenahelper.AppendA(c.trees, t, a)
	return res, t
}
