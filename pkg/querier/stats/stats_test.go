// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/querier/stats/stats_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package stats

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStats_WallTime(t *testing.T) {
	t.Run("add and load wall time", func(t *testing.T) {
		stats, _ := ContextWithEmptyStats(context.Background())
		stats.AddWallTime(time.Second)
		stats.AddWallTime(time.Second)

		assert.Equal(t, 2*time.Second, stats.LoadWallTime())
	})

	t.Run("add and load wall time nil receiver", func(t *testing.T) {
		var stats *Stats
		stats.AddWallTime(time.Second)

		assert.Equal(t, time.Duration(0), stats.LoadWallTime())
	})
}

func TestStats_AddFetchedSeries(t *testing.T) {
	t.Run("add and load series", func(t *testing.T) {
		stats, _ := ContextWithEmptyStats(context.Background())
		stats.AddFetchedSeries(100)
		stats.AddFetchedSeries(50)

		assert.Equal(t, uint64(150), stats.LoadFetchedSeries())
	})

	t.Run("add and load series nil receiver", func(t *testing.T) {
		var stats *Stats
		stats.AddFetchedSeries(50)

		assert.Equal(t, uint64(0), stats.LoadFetchedSeries())
	})
}

func TestStats_AddFetchedChunkBytes(t *testing.T) {
	t.Run("add and load bytes", func(t *testing.T) {
		stats, _ := ContextWithEmptyStats(context.Background())
		stats.AddFetchedChunkBytes(4096)
		stats.AddFetchedChunkBytes(4096)

		assert.Equal(t, uint64(8192), stats.LoadFetchedChunkBytes())
	})

	t.Run("add and load bytes nil receiver", func(t *testing.T) {
		var stats *Stats
		stats.AddFetchedChunkBytes(1024)

		assert.Equal(t, uint64(0), stats.LoadFetchedChunkBytes())
	})
}

func TestStats_AddFetchedChunks(t *testing.T) {
	t.Run("add and load chunks", func(t *testing.T) {
		stats, _ := ContextWithEmptyStats(context.Background())
		stats.AddFetchedChunks(20)
		stats.AddFetchedChunks(22)

		assert.Equal(t, uint64(42), stats.LoadFetchedChunks())
	})

	t.Run("add and load chunks nil receiver", func(t *testing.T) {
		var stats *Stats
		stats.AddFetchedChunks(3)

		assert.Equal(t, uint64(0), stats.LoadFetchedChunks())
	})
}

func TestStats_AddShardedQueries(t *testing.T) {
	t.Run("add and load sharded queries", func(t *testing.T) {
		stats, _ := ContextWithEmptyStats(context.Background())
		stats.AddShardedQueries(20)
		stats.AddShardedQueries(22)

		assert.Equal(t, uint32(42), stats.LoadShardedQueries())
	})

	t.Run("add and load sharded queries nil receiver", func(t *testing.T) {
		var stats *Stats
		stats.AddShardedQueries(3)

		assert.Equal(t, uint32(0), stats.LoadShardedQueries())
	})
}

func TestStats_AddSplitQueries(t *testing.T) {
	t.Run("add and load split queries", func(t *testing.T) {
		stats, _ := ContextWithEmptyStats(context.Background())
		stats.AddSplitQueries(10)
		stats.AddSplitQueries(11)

		assert.Equal(t, uint32(21), stats.LoadSplitQueries())
	})

	t.Run("add and load split queries nil receiver", func(t *testing.T) {
		var stats *Stats
		stats.AddSplitQueries(1)

		assert.Equal(t, uint32(0), stats.LoadSplitQueries())
	})
}

func TestStats_Merge(t *testing.T) {
	t.Run("merge two stats objects", func(t *testing.T) {
		stats1 := &Stats{}
		stats1.AddWallTime(time.Millisecond)
		stats1.AddFetchedSeries(50)
		stats1.AddFetchedChunkBytes(42)
		stats1.AddFetchedChunks(10)
		stats1.AddShardedQueries(20)
		stats1.AddSplitQueries(10)

		stats2 := &Stats{}
		stats2.AddWallTime(time.Second)
		stats2.AddFetchedSeries(60)
		stats2.AddFetchedChunkBytes(100)
		stats2.AddFetchedChunks(11)
		stats2.AddShardedQueries(21)
		stats2.AddSplitQueries(11)

		stats1.Merge(stats2)

		assert.Equal(t, 1001*time.Millisecond, stats1.LoadWallTime())
		assert.Equal(t, uint64(110), stats1.LoadFetchedSeries())
		assert.Equal(t, uint64(142), stats1.LoadFetchedChunkBytes())
		assert.Equal(t, uint64(21), stats1.LoadFetchedChunks())
		assert.Equal(t, uint32(41), stats1.LoadShardedQueries())
		assert.Equal(t, uint32(21), stats1.LoadSplitQueries())
	})

	t.Run("merge two nil stats objects", func(t *testing.T) {
		var stats1 *Stats
		var stats2 *Stats

		stats1.Merge(stats2)

		assert.Equal(t, time.Duration(0), stats1.LoadWallTime())
		assert.Equal(t, uint64(0), stats1.LoadFetchedSeries())
		assert.Equal(t, uint64(0), stats1.LoadFetchedChunkBytes())
		assert.Equal(t, uint64(0), stats1.LoadFetchedChunks())
		assert.Equal(t, uint32(0), stats1.LoadShardedQueries())
		assert.Equal(t, uint32(0), stats1.LoadSplitQueries())
	})
}
