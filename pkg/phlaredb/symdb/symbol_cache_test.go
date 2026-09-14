package symdb

import (
	"context"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

// openReaderWithCache flushes the mem suite and opens a Reader with the cache.
func openReaderWithCache(t testing.TB, s *memSuite, cache *SymbolCache) *Reader {
	require.NoError(t, s.db.Flush())
	b, err := filesystem.NewBucket(s.config.Dir)
	require.NoError(t, err)
	r, err := Open(context.Background(), b, &block.Meta{Files: s.db.Files()}, WithSymbolCache(cache))
	require.NoError(t, err)
	return r
}

// Test_SymbolCache_ConsecutiveQueriesReuseUntilClose models the store-gateway
// lifecycle: a block's Reader is opened once and reused across many queries, and
// purged only when the block is dropped (Reader.Close). N consecutive queries
// yield exactly one decode (miss) and N-1 warm hits, each hit reusing the very
// same backing array (no re-decode); the cache is purged only at close.
func Test_SymbolCache_ConsecutiveQueriesReuseUntilClose(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	cache := NewSymbolCache(1<<30, prometheus.NewRegistry())
	r := openReaderWithCache(t, s, cache)

	const n = 5
	var first *schemav1.InMemoryFunction
	for i := 0; i < n; i++ {
		pr, err := r.Partition(context.Background(), 0)
		require.NoError(t, err)
		fns := pr.Symbols().Functions
		require.NotEmpty(t, fns, "block must have decoded functions")
		if i == 0 {
			first = &fns[0]
		} else {
			require.Same(t, first, &fns[0], "warm query reuses the same backing array (no re-decode)")
		}
		pr.Release() // query finishes; block Reader stays open (as in prod)
	}
	require.Equal(t, float64(1), testutil.ToFloat64(cache.metrics.misses), "only the first query decodes")
	require.Equal(t, float64(n-1), testutil.ToFloat64(cache.metrics.hits), "consecutive queries are warm hits")
	require.Greater(t, testutil.ToFloat64(cache.metrics.sizeBytes), float64(0), "entry stays resident across queries")

	// The cache is purged only when the block Reader is closed (block drop).
	require.NoError(t, r.Close())
	require.Equal(t, float64(0), testutil.ToFloat64(cache.metrics.sizeBytes), "cache purged on block close")
}

// Test_SymbolCache_MissTransfersOwnershipToCache asserts the snapshot-and-retain
// contract: on a miss the decoded slices are handed to the cache entry and the
// table wrappers are released (t.s nil'd), yet the backing arrays survive the
// release and remain valid/readable via the cache.
func Test_SymbolCache_MissTransfersOwnershipToCache(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	r := openReaderWithCache(t, s, NewSymbolCache(1<<30, prometheus.NewRegistry()))
	defer r.Close()

	pr, err := r.Partition(context.Background(), 0)
	require.NoError(t, err)
	p := pr.(*partition)

	// Cache entry owns the decoded data...
	require.NotNil(t, p.cached)
	require.NotEmpty(t, p.cached.functions, "cache entry holds decoded functions")
	require.NotEmpty(t, p.cached.strings, "cache entry holds decoded strings")
	// ...and the table wrappers were released (their slice headers nil'd).
	require.Empty(t, p.locations.slice(), "table released after snapshot")
	require.Empty(t, p.functions.slice(), "table released after snapshot")
	require.Empty(t, p.strings.slice(), "table released after snapshot")

	// Symbols() serves from the cache, and the backing arrays survive the
	// release: read the tail elements to confirm the memory is live and valid.
	syms := p.Symbols()
	require.Equal(t, p.cached.functions, syms.Functions)
	require.NotZero(t, syms.Functions[len(syms.Functions)-1].Name+1)
	require.NotEmpty(t, syms.Strings[len(syms.Strings)-1])
	pr.Release()
}

// Test_SymbolCache_ConcurrentLoadOnce runs many concurrent queries against the
// same partition and asserts (under -race) that they share a single decode: the
// symRef load-once barrier + the cache mutex must serialize correctly, yielding
// exactly one miss regardless of concurrency.
func Test_SymbolCache_ConcurrentLoadOnce(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	cache := NewSymbolCache(1<<30, prometheus.NewRegistry())
	r := openReaderWithCache(t, s, cache)
	defer r.Close()

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			pr, err := r.Partition(context.Background(), 0)
			require.NoError(t, err)
			require.NotEmpty(t, pr.Symbols().Functions)
			pr.Release()
		}()
	}
	wg.Wait()

	// Only the very first fetch decodes; every other fetch (even after the
	// refcount cycles) finds the entry resident, so misses stays at exactly 1.
	require.Equal(t, float64(1), testutil.ToFloat64(cache.metrics.misses))
}

func fnEntry(n int) *cachedSymbols {
	return newCachedSymbols(nil, nil, make([]schemav1.InMemoryFunction, n), nil)
}

// Test_SymbolCache_EvictsOverBudget checks LRU byte-budget eviction drives the
// resident set back under maxBytes.
func Test_SymbolCache_EvictsOverBudget(t *testing.T) {
	c := NewSymbolCache(5000, prometheus.NewRegistry())
	kA := SymbolCacheKey{Partition: 1}
	kB := SymbolCacheKey{Partition: 2}
	kC := SymbolCacheKey{Partition: 3}

	c.add(kA, fnEntry(100)) // ~2496 bytes
	c.add(kB, fnEntry(100)) // ~4992 < 5000
	c.add(kC, fnEntry(100)) // ~7488 > 5000 -> evict oldest (A)

	require.Nil(t, c.get(kA), "oldest entry should have been evicted")
	require.NotNil(t, c.get(kC), "newest entry must survive")
	require.LessOrEqual(t, c.curBytes, c.maxBytes, "resident set is bounded by maxBytes")
}

// Test_SymbolCache_PurgeBlock removes only the target block's entries.
func Test_SymbolCache_PurgeBlock(t *testing.T) {
	c := NewSymbolCache(1<<30, prometheus.NewRegistry())
	blockX := block.Meta{}.ULID // zero ULID
	blockY := blockX
	blockY[0] = 1
	kX1 := SymbolCacheKey{Block: blockX, Partition: 1}
	kX2 := SymbolCacheKey{Block: blockX, Partition: 2}
	kY1 := SymbolCacheKey{Block: blockY, Partition: 1}
	for _, k := range []SymbolCacheKey{kX1, kX2, kY1} {
		c.add(k, fnEntry(10))
	}
	c.PurgeBlock(blockX)
	require.Nil(t, c.get(kX1))
	require.Nil(t, c.get(kX2))
	require.NotNil(t, c.get(kY1), "other block's entry must remain")
	require.Equal(t, fnEntry(10).bytes, c.curBytes, "budget reflects only the surviving entry")
}
