//go:build !goexperiment.arenas

package streaming

import "github.com/grafana/pyroscope/pkg/og/storage/tree"

func (c *LabelsCache) newCacheEntryA(l Labels) (int, *tree.Tree) {
	return c.newCacheEntry(l)
}
