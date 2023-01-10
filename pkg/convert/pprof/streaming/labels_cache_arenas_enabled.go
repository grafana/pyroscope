//go:build goexperiment.arenas

package streaming

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util"
)

func (c *LabelsCache) newCacheEntryA(l Labels) (int, *tree.Tree) {
	if c.arena == nil {
		return c.newCacheEntry(l)
	}
	from := len(c.labels)
	for _, u := range l {//todo all appends to arena
		c.labels = util.AppendA(c.labels, u, c.arena)
	}
	to := len(c.labels)
	res := len(c.labelRefs)
	c.labelRefs = util.AppendA(c.labelRefs, uint64(from<<32|to), c.arena)
	t := tree.NewA(c.arena)
	c.trees = util.AppendA(c.trees, t, c.arena)
	return res, t
}
