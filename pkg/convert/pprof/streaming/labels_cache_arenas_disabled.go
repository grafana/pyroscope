//go:build !goexperiment.arenas

package streaming

import "github.com/pyroscope-io/pyroscope/pkg/storage/tree"

func (c *LabelsCache) newCacheEntryA(l Labels) (int, *tree.Tree) {
	return c.newCacheEntry(l)
}
