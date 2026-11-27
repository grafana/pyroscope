package block

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/util"
	"github.com/grafana/pyroscope/v2/pkg/util/bufferpool"
)

const (
	// parquetCoalesceMaxGap bridges adjacent/nearby column chunks within or across
	// row groups (e.g. skipping tiny unused metadata or sample columns) to collapse
	// multiple GetRange RTTs into a single range read.
	parquetCoalesceMaxGap = 256 << 10
	// parquetCoalesceMaxRange caps a single coalesced range read so memory stays
	// bounded and concurrent workers can fetch chunks in parallel.
	parquetCoalesceMaxRange = 8 << 20
	parquetPrefetchWorkers  = 8
)

type cachedSpan struct {
	off  int64
	size int64
	buf  *bufferpool.Buffer

	once sync.Once
	data []byte
	err  error
}

// coalescedSectionReader serves prefetched/coalesced byte spans in [0, size)
// on demand with singleflight-per-span deduplication, falling back to the
// underlying section reader for any uncached range.
type coalescedSectionReader struct {
	fallback io.ReaderAt
	storage  objstore.BucketReader
	path     string
	baseOff  int64

	mu    sync.RWMutex
	spans []*cachedSpan
}

func newCoalescedSectionReader(
	fallback io.ReaderAt,
	storage objstore.BucketReader,
	path string,
	baseOff int64,
) *coalescedSectionReader {
	return &coalescedSectionReader{
		fallback: fallback,
		storage:  storage,
		path:     path,
		baseOff:  baseOff,
	}
}

func (r *coalescedSectionReader) lookupSpan(off, length int64) *cachedSpan {
	r.mu.RLock()
	spans := r.spans
	r.mu.RUnlock()
	if len(spans) == 0 {
		return nil
	}
	// Binary search for the last span with span.off <= off.
	lo, hi := 0, len(spans)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if spans[mid].off <= off {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return nil
	}
	sp := spans[lo-1]
	if off >= sp.off && off+length <= sp.off+sp.size {
		return sp
	}
	return nil
}

func (r *coalescedSectionReader) fetchSpan(ctx context.Context, sp *cachedSpan) ([]byte, error) {
	sp.once.Do(func() {
		buf := bufferpool.GetBuffer(int(sp.size))
		if err := objstore.ReadRange(ctx, buf, r.path, r.storage, r.baseOff+sp.off, sp.size); err != nil {
			bufferpool.Put(buf)
			sp.err = err
			return
		}
		sp.buf = buf
		sp.data = buf.B
	})
	return sp.data, sp.err
}

func (r *coalescedSectionReader) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if sp := r.lookupSpan(off, int64(len(p))); sp != nil {
		data, err := r.fetchSpan(context.Background(), sp)
		if err != nil {
			return 0, err
		}
		rel := off - sp.off
		return copy(p, data[rel:rel+int64(len(p))]), nil
	}
	return r.fallback.ReadAt(p, off)
}

func (r *coalescedSectionReader) close() {
	r.mu.Lock()
	spans := r.spans
	r.spans = nil
	r.mu.Unlock()
	for _, sp := range spans {
		sp.once.Do(func() {})
		if sp.buf != nil {
			bufferpool.Put(sp.buf)
			sp.buf = nil
			sp.data = nil
		}
	}
}

type byteRange struct {
	lo int64
	hi int64
}

// PrefetchColumns coalesces and prefetches the byte ranges of the specified
// columns across the matching row groups. When profile Parquet tables are
// already backed by an in-memory buffer or local disk, PrefetchColumns is a no-op.
//
// Callers may invoke PrefetchColumns once the set of candidate row groups and
// needed column indices are known so all column chunks are fetched in minimal
// coalesced concurrent GetRange calls rather than per-column-per-RG ReadAt calls.
func (f *ParquetFile) PrefetchColumns(ctx context.Context, rowGroups []int, columns []int) error {
	if f == nil || f.coalescer == nil || len(rowGroups) == 0 || len(columns) == 0 {
		return nil
	}
	meta := f.Metadata()
	if meta == nil || len(meta.RowGroups) == 0 {
		return nil
	}

	raw := make([]byteRange, 0, len(rowGroups)*len(columns))
	for _, rgIdx := range rowGroups {
		if rgIdx < 0 || rgIdx >= len(meta.RowGroups) {
			continue
		}
		rgCols := meta.RowGroups[rgIdx].Columns
		for _, colIdx := range columns {
			if colIdx < 0 || colIdx >= len(rgCols) {
				continue
			}
			cc := rgCols[colIdx].MetaData
			if cc.TotalCompressedSize <= 0 {
				continue
			}
			lo := cc.DataPageOffset
			if cc.DictionaryPageOffset > 0 && cc.DictionaryPageOffset < lo {
				lo = cc.DictionaryPageOffset
			}
			hi := lo + cc.TotalCompressedSize
			raw = append(raw, byteRange{lo: lo, hi: hi})
		}
	}
	if len(raw) == 0 {
		return nil
	}

	slices.SortFunc(raw, func(a, b byteRange) int {
		if a.lo != b.lo {
			if a.lo < b.lo {
				return -1
			}
			return 1
		}
		if a.hi < b.hi {
			return -1
		}
		if a.hi > b.hi {
			return 1
		}
		return 0
	})

	merged := raw[:0]
	for _, cur := range raw {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			if cur.lo <= last.hi {
				if cur.hi > last.hi {
					last.hi = cur.hi
				}
				continue
			}
			maxHi := max(last.hi, cur.hi)
			if cur.lo-last.hi <= parquetCoalesceMaxGap && maxHi-last.lo <= parquetCoalesceMaxRange {
				last.hi = maxHi
				continue
			}
		}
		merged = append(merged, cur)
	}

	f.coalescer.mu.Lock()
	if len(f.coalescer.spans) > 0 {
		// Already initialized for this file; merge any non-overlapping new spans
		// or skip if covered. In BlockReader each dataset runs one query pipeline,
		// so spans is typically empty here.
		existing := f.coalescer.spans
		var toFetch []*cachedSpan
		all := make([]*cachedSpan, 0, len(existing)+len(merged))
		all = append(all, existing...)
		for _, m := range merged {
			covered := false
			for _, ex := range existing {
				if m.lo >= ex.off && m.hi <= ex.off+ex.size {
					covered = true
					break
				}
			}
			if !covered {
				// Avoid overlapping spans so binary search stays monotonic.
				overlaps := false
				for _, ex := range existing {
					if m.lo < ex.off+ex.size && m.hi > ex.off {
						overlaps = true
						break
					}
				}
				if !overlaps {
					sp := &cachedSpan{off: m.lo, size: m.hi - m.lo}
					all = append(all, sp)
					toFetch = append(toFetch, sp)
				}
			}
		}
		slices.SortFunc(all, func(a, b *cachedSpan) int {
			if a.off < b.off {
				return -1
			}
			if a.off > b.off {
				return 1
			}
			return 0
		})
		f.coalescer.spans = all
		f.coalescer.mu.Unlock()
		return f.coalescer.prefetchSpans(ctx, toFetch)
	}

	spans := make([]*cachedSpan, len(merged))
	for i, m := range merged {
		spans[i] = &cachedSpan{off: m.lo, size: m.hi - m.lo}
	}
	f.coalescer.spans = spans
	f.coalescer.mu.Unlock()

	return f.coalescer.prefetchSpans(ctx, spans)
}

func (r *coalescedSectionReader) prefetchSpans(ctx context.Context, spans []*cachedSpan) error {
	if len(spans) == 0 {
		return nil
	}
	if len(spans) == 1 {
		if _, err := r.fetchSpan(ctx, spans[0]); err != nil {
			return fmt.Errorf("prefetching parquet column range: %w", err)
		}
		return nil
	}
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(parquetPrefetchWorkers)
	for _, sp := range spans {
		sp := sp
		g.Go(util.RecoverPanic(func() error {
			if _, err := r.fetchSpan(ctx, sp); err != nil {
				return fmt.Errorf("prefetching parquet column range: %w", err)
			}
			return nil
		}))
	}
	return g.Wait()
}
