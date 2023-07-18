//go:build goexperiment.arenas

package streaming

import (
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/og/util/arenahelper"
)

func (c *LabelsCache) newCacheEntryA(l Labels) (int, *tree.Tree) {
	a := c.arena
	if a == nil {
		return c.newCacheEntry(l)
	}
	from := len(c.labels)
	for _, u := range l {
		if len(c.labels) < cap(c.labels) {
			c.labels = append(c.labels, u)
		} else {
			c.labels = arenahelper.AppendA(c.labels, u, a)
		}
	}
	to := len(c.labels)
	res := len(c.labelRefs)
	r := uint64(from<<32 | to)
	if len(c.labelRefs) < cap(c.labelRefs) {
		c.labelRefs = append(c.labelRefs, r)
	} else {
		c.labelRefs = arenahelper.AppendA(c.labelRefs, r, a)
	}
	t := tree.NewA(a)
	if len(c.trees) < cap(c.trees) {
		c.trees = append(c.trees, t)
	} else {
		c.trees = arenahelper.AppendA(c.trees, t, a)
	}
	return res, t
}
